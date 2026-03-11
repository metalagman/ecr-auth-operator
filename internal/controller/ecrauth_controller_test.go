/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"errors"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	ecrv1alpha1 "github.com/metalagman/ecr-auth-operator/api/v1alpha1"
)

type fakeTokenProvider struct {
	tokens []ECRAuthorizationToken
	err    error
}

func (f *fakeTokenProvider) GetAuthorizationTokens(_ context.Context, _ ecrv1alpha1.ECRAuthSpec) ([]ECRAuthorizationToken, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.tokens, nil
}

func readyCondition(resource *ecrv1alpha1.ECRAuth) *metav1.Condition {
	for i := range resource.Status.Conditions {
		if resource.Status.Conditions[i].Type == conditionTypeReady {
			return &resource.Status.Conditions[i]
		}
	}
	return nil
}

var _ = Describe("ECRAuth Controller", func() {
	var (
		ctx        context.Context
		namespace  string
		fixedNow   time.Time
		baseTokens []ECRAuthorizationToken
		reconciler *ECRAuthReconciler
	)

	BeforeEach(func() {
		ctx = context.Background()
		fixedNow = time.Date(2026, 3, 11, 7, 0, 0, 0, time.UTC)
		baseTokens = []ECRAuthorizationToken{
			{
				ProxyEndpoint: "https://123456789012.dkr.ecr.us-east-1.amazonaws.com",
				Username:      "AWS",
				Password:      "token-password",
			},
			{
				ProxyEndpoint: "https://210987654321.dkr.ecr.eu-west-1.amazonaws.com",
				Username:      "AWS",
				Password:      "token-password-2",
			},
		}

		ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{GenerateName: "ecrauth-test-"}}
		Expect(k8sClient.Create(ctx, ns)).To(Succeed())
		namespace = ns.Name

		reconciler = &ECRAuthReconciler{
			Client:        k8sClient,
			Scheme:        k8sClient.Scheme(),
			TokenProvider: &fakeTokenProvider{tokens: baseTokens},
			Now: func() time.Time {
				return fixedNow
			},
		}
	})

	AfterEach(func() {
		ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}
		_ = k8sClient.Delete(ctx, ns)
	})

	It("creates a managed dockerconfigjson secret and updates Ready condition", func() {
		resource := &ecrv1alpha1.ECRAuth{
			ObjectMeta: metav1.ObjectMeta{Name: "auth-a", Namespace: namespace},
			Spec: ecrv1alpha1.ECRAuthSpec{
				SecretName: "regcred",
				Registries: []string{
					"123456789012.dkr.ecr.us-east-1.amazonaws.com",
					"210987654321.dkr.ecr.eu-west-1.amazonaws.com",
				},
			},
		}
		Expect(k8sClient.Create(ctx, resource)).To(Succeed())

		result, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: types.NamespacedName{
			Namespace: namespace,
			Name:      resource.Name,
		}})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.RequeueAfter).To(Equal(defaultRefreshInterval))

		secret := &corev1.Secret{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: "regcred"}, secret)).To(Succeed())
		Expect(secret.Type).To(Equal(corev1.SecretTypeDockerConfigJson))
		Expect(secret.Data).To(HaveKey(dockerConfigJSONKey))
		Expect(secret.Labels).To(HaveKeyWithValue(managedByLabelKey, managedByLabelValue))
		Expect(secret.Annotations).To(HaveKey(ownerUIDAnnotation))
		Expect(string(secret.Data[dockerConfigJSONKey])).To(ContainSubstring("123456789012.dkr.ecr.us-east-1.amazonaws.com"))
		Expect(string(secret.Data[dockerConfigJSONKey])).To(ContainSubstring("210987654321.dkr.ecr.eu-west-1.amazonaws.com"))

		updated := &ecrv1alpha1.ECRAuth{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: resource.Name}, updated)).To(Succeed())
		cond := readyCondition(updated)
		Expect(cond).NotTo(BeNil())
		Expect(cond.Status).To(Equal(metav1.ConditionTrue))
		Expect(cond.Reason).To(Equal(reasonReconciled))
		Expect(updated.Status.ManagedSecretName).To(Equal("regcred"))
		Expect(updated.Status.LastSuccessfulRefreshTime).NotTo(BeNil())
	})

	It("fails with conflict when target secret exists and is unmanaged", func() {
		foreign := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "regcred", Namespace: namespace},
			Type:       corev1.SecretTypeOpaque,
			Data:       map[string][]byte{"x": []byte("y")},
		}
		Expect(k8sClient.Create(ctx, foreign)).To(Succeed())

		resource := &ecrv1alpha1.ECRAuth{
			ObjectMeta: metav1.ObjectMeta{Name: "auth-b", Namespace: namespace},
			Spec: ecrv1alpha1.ECRAuthSpec{
				SecretName: "regcred",
				Registries: []string{"123456789012.dkr.ecr.us-east-1.amazonaws.com"},
			},
		}
		Expect(k8sClient.Create(ctx, resource)).To(Succeed())

		result, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: types.NamespacedName{
			Namespace: namespace,
			Name:      resource.Name,
		}})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.RequeueAfter).To(Equal(defaultRefreshInterval))

		updated := &ecrv1alpha1.ECRAuth{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: resource.Name}, updated)).To(Succeed())
		cond := readyCondition(updated)
		Expect(cond).NotTo(BeNil())
		Expect(cond.Status).To(Equal(metav1.ConditionFalse))
		Expect(cond.Reason).To(Equal(reasonSecretConflict))

		persisted := &corev1.Secret{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: "regcred"}, persisted)).To(Succeed())
		Expect(persisted.Type).To(Equal(corev1.SecretTypeOpaque))
	})

	It("rejects second ECRAuth managing same secret", func() {
		primary := &ecrv1alpha1.ECRAuth{
			ObjectMeta: metav1.ObjectMeta{Name: "auth-c1", Namespace: namespace},
			Spec:       ecrv1alpha1.ECRAuthSpec{SecretName: "regcred", Registries: []string{"123456789012.dkr.ecr.us-east-1.amazonaws.com"}},
		}
		secondary := &ecrv1alpha1.ECRAuth{
			ObjectMeta: metav1.ObjectMeta{Name: "auth-c2", Namespace: namespace},
			Spec:       ecrv1alpha1.ECRAuthSpec{SecretName: "regcred", Registries: []string{"123456789012.dkr.ecr.us-east-1.amazonaws.com"}},
		}

		Expect(k8sClient.Create(ctx, primary)).To(Succeed())
		_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: types.NamespacedName{
			Namespace: namespace,
			Name:      primary.Name,
		}})
		Expect(err).NotTo(HaveOccurred())

		Expect(k8sClient.Create(ctx, secondary)).To(Succeed())
		result, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: types.NamespacedName{
			Namespace: namespace,
			Name:      secondary.Name,
		}})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.RequeueAfter).To(Equal(defaultRefreshInterval))

		updated := &ecrv1alpha1.ECRAuth{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: secondary.Name}, updated)).To(Succeed())
		cond := readyCondition(updated)
		Expect(cond).NotTo(BeNil())
		Expect(cond.Status).To(Equal(metav1.ConditionFalse))
		Expect(cond.Reason).To(Equal(reasonDuplicateSecretName))
	})

	It("uses custom refresh interval", func() {
		resource := &ecrv1alpha1.ECRAuth{
			ObjectMeta: metav1.ObjectMeta{Name: "auth-d", Namespace: namespace},
			Spec: ecrv1alpha1.ECRAuthSpec{
				SecretName:      "regcred-custom",
				Registries:      []string{"123456789012.dkr.ecr.us-east-1.amazonaws.com"},
				RefreshInterval: &metav1.Duration{Duration: 2 * time.Hour},
			},
		}
		Expect(k8sClient.Create(ctx, resource)).To(Succeed())

		result, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: types.NamespacedName{
			Namespace: namespace,
			Name:      resource.Name,
		}})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.RequeueAfter).To(Equal(2 * time.Hour))
	})

	It("sets auth error condition when token provider fails", func() {
		reconciler.TokenProvider = &fakeTokenProvider{err: errors.New("access denied")}

		resource := &ecrv1alpha1.ECRAuth{
			ObjectMeta: metav1.ObjectMeta{Name: "auth-e", Namespace: namespace},
			Spec: ecrv1alpha1.ECRAuthSpec{
				SecretName: "regcred",
				Registries: []string{"123456789012.dkr.ecr.us-east-1.amazonaws.com"},
			},
		}
		Expect(k8sClient.Create(ctx, resource)).To(Succeed())

		result, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: types.NamespacedName{
			Namespace: namespace,
			Name:      resource.Name,
		}})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.RequeueAfter).To(Equal(authErrorRetryInterval))

		updated := &ecrv1alpha1.ECRAuth{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: resource.Name}, updated)).To(Succeed())
		cond := readyCondition(updated)
		Expect(cond).NotTo(BeNil())
		Expect(cond.Status).To(Equal(metav1.ConditionFalse))
		Expect(cond.Reason).To(Equal(reasonAuthFetchFailed))
	})
})
