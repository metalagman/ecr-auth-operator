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
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	ecrv1alpha1 "github.com/metalagman/ecr-auth-operator/api/v1alpha1"
)

const (
	dockerConfigJSONKey = ".dockerconfigjson"

	managedByLabelKey   = "ecr.metalagman.dev/managed-by"
	managedByLabelValue = "ecr-auth-operator"
	ownerUIDAnnotation  = "ecr.metalagman.dev/owner-uid"

	conditionTypeReady = "Ready"

	reasonValidationFailed    = "ValidationFailed"
	reasonSecretConflict      = "SecretConflict"
	reasonDuplicateSecretName = "DuplicateSecretName"
	reasonAuthFetchFailed     = "AuthFetchFailed"
	reasonSecretWriteFailed   = "SecretWriteFailed"
	reasonStatusUpdateFailed  = "StatusUpdateFailed"
	reasonReconciled          = "Reconciled"

	defaultRefreshInterval = 11 * time.Hour
	authErrorRetryInterval = 1 * time.Minute
)

var errConditionUpdateFailed = errors.New("condition update failed")

// ECRTokenProvider fetches docker auth credentials from ECR.
type ECRTokenProvider interface {
	GetAuthorizationTokens(ctx context.Context, spec ecrv1alpha1.ECRAuthSpec) ([]ECRAuthorizationToken, error)
}

// ECRAuthorizationToken is normalized ECR token data used to build docker config.
type ECRAuthorizationToken struct {
	ProxyEndpoint string
	Username      string
	Password      string
	ExpiresAt     *time.Time
}

// ECRAuthReconciler reconciles a ECRAuth object.
type ECRAuthReconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	TokenProvider ECRTokenProvider
	Now           func() time.Time
}

// +kubebuilder:rbac:groups=ecr.metalagman.dev,resources=ecrauths,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=ecr.metalagman.dev,resources=ecrauths/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=ecr.metalagman.dev,resources=ecrauths/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

// Reconcile moves cluster state towards the desired ECRAuth state.
func (r *ECRAuthReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)

	nowFn := r.Now
	if nowFn == nil {
		nowFn = time.Now
	}

	refreshProvider := r.TokenProvider
	if refreshProvider == nil {
		return ctrl.Result{}, fmt.Errorf("token provider is not configured")
	}

	var auth ecrv1alpha1.ECRAuth
	if err := r.Get(ctx, req.NamespacedName, &auth); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !auth.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, nil
	}

	refreshInterval := resolveRefreshInterval(auth.Spec.RefreshInterval)
	if err := validateSpec(auth.Spec); err != nil {
		if statusErr := r.setCondition(ctx, &auth, metav1.ConditionFalse, reasonValidationFailed, err.Error(), nil); statusErr != nil {
			return ctrl.Result{}, fmt.Errorf("%w: %v", errConditionUpdateFailed, statusErr)
		}
		return ctrl.Result{}, nil
	}

	if err := r.cleanupPreviousSecretIfNeeded(ctx, &auth); err != nil {
		if statusErr := r.setCondition(ctx, &auth, metav1.ConditionFalse, reasonSecretWriteFailed, err.Error(), nil); statusErr != nil {
			return ctrl.Result{}, fmt.Errorf("%w: %v", errConditionUpdateFailed, statusErr)
		}
		return ctrl.Result{RequeueAfter: refreshInterval}, nil
	}

	secretKey := types.NamespacedName{Namespace: auth.Namespace, Name: auth.Spec.SecretName}
	existingSecret := &corev1.Secret{}
	err := r.Get(ctx, secretKey, existingSecret)
	if err != nil && !apierrors.IsNotFound(err) {
		return ctrl.Result{}, err
	}

	secretExists := err == nil
	if secretExists {
		if !isManagedSecret(existingSecret) {
			msg := fmt.Sprintf("target secret %q exists but is not managed by this operator", auth.Spec.SecretName)
			if statusErr := r.setCondition(ctx, &auth, metav1.ConditionFalse, reasonSecretConflict, msg, nil); statusErr != nil {
				return ctrl.Result{}, fmt.Errorf("%w: %v", errConditionUpdateFailed, statusErr)
			}
			return ctrl.Result{RequeueAfter: refreshInterval}, nil
		}

		if !secretOwnedBy(existingSecret, &auth) {
			msg := fmt.Sprintf("target secret %q is already managed by another ECRAuth", auth.Spec.SecretName)
			if statusErr := r.setCondition(ctx, &auth, metav1.ConditionFalse, reasonDuplicateSecretName, msg, nil); statusErr != nil {
				return ctrl.Result{}, fmt.Errorf("%w: %v", errConditionUpdateFailed, statusErr)
			}
			return ctrl.Result{RequeueAfter: refreshInterval}, nil
		}
	}

	tokens, err := refreshProvider.GetAuthorizationTokens(ctx, auth.Spec)
	if err != nil {
		msg := fmt.Sprintf("failed to fetch ECR authorization tokens: %v", err)
		if statusErr := r.setCondition(ctx, &auth, metav1.ConditionFalse, reasonAuthFetchFailed, msg, nil); statusErr != nil {
			return ctrl.Result{}, fmt.Errorf("%w: %v", errConditionUpdateFailed, statusErr)
		}
		logger.Error(err, "failed to get authorization tokens")
		return ctrl.Result{RequeueAfter: minDuration(refreshInterval, authErrorRetryInterval)}, nil
	}

	dockerConfigJSON, err := buildDockerConfigJSON(tokens)
	if err != nil {
		if statusErr := r.setCondition(ctx, &auth, metav1.ConditionFalse, reasonAuthFetchFailed, err.Error(), nil); statusErr != nil {
			return ctrl.Result{}, fmt.Errorf("%w: %v", errConditionUpdateFailed, statusErr)
		}
		return ctrl.Result{RequeueAfter: minDuration(refreshInterval, authErrorRetryInterval)}, nil
	}

	if err := r.upsertManagedSecret(ctx, &auth, existingSecret, secretExists, dockerConfigJSON); err != nil {
		msg := fmt.Sprintf("failed to persist managed secret: %v", err)
		if statusErr := r.setCondition(ctx, &auth, metav1.ConditionFalse, reasonSecretWriteFailed, msg, nil); statusErr != nil {
			return ctrl.Result{}, fmt.Errorf("%w: %v", errConditionUpdateFailed, statusErr)
		}
		return ctrl.Result{RequeueAfter: refreshInterval}, nil
	}

	now := metav1.NewTime(nowFn().UTC())
	if err := r.setCondition(ctx, &auth, metav1.ConditionTrue, reasonReconciled, "managed secret refreshed", &now); err != nil {
		return ctrl.Result{}, fmt.Errorf("%w: %v", errConditionUpdateFailed, err)
	}

	return reconcile.Result{RequeueAfter: refreshInterval}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ECRAuthReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&ecrv1alpha1.ECRAuth{}).
		Owns(&corev1.Secret{}).
		Named("ecrauth").
		Complete(r)
}

func (r *ECRAuthReconciler) cleanupPreviousSecretIfNeeded(ctx context.Context, auth *ecrv1alpha1.ECRAuth) error {
	previousSecret := strings.TrimSpace(auth.Status.ManagedSecretName)
	if previousSecret == "" || previousSecret == auth.Spec.SecretName {
		return nil
	}

	secret := &corev1.Secret{}
	secretKey := types.NamespacedName{Namespace: auth.Namespace, Name: previousSecret}
	if err := r.Get(ctx, secretKey, secret); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}

	if !isManagedSecret(secret) || !secretOwnedBy(secret, auth) {
		return nil
	}

	if err := r.Delete(ctx, secret); err != nil && !apierrors.IsNotFound(err) {
		return err
	}

	return nil
}

func (r *ECRAuthReconciler) upsertManagedSecret(
	ctx context.Context,
	auth *ecrv1alpha1.ECRAuth,
	existing *corev1.Secret,
	secretExists bool,
	dockerConfigJSON []byte,
) error {
	if !secretExists {
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: auth.Namespace,
				Name:      auth.Spec.SecretName,
			},
		}
		setManagedSecretMetadata(secret, auth)
		secret.Type = corev1.SecretTypeDockerConfigJson
		secret.Data = map[string][]byte{dockerConfigJSONKey: dockerConfigJSON}

		if err := controllerutil.SetControllerReference(auth, secret, r.Scheme); err != nil {
			return err
		}
		return r.Create(ctx, secret)
	}

	updated := existing.DeepCopy()
	setManagedSecretMetadata(updated, auth)
	updated.Type = corev1.SecretTypeDockerConfigJson
	if updated.Data == nil {
		updated.Data = map[string][]byte{}
	}
	updated.Data[dockerConfigJSONKey] = dockerConfigJSON
	if err := controllerutil.SetControllerReference(auth, updated, r.Scheme); err != nil {
		return err
	}

	if reflect.DeepEqual(updated.Data, existing.Data) &&
		reflect.DeepEqual(updated.Labels, existing.Labels) &&
		reflect.DeepEqual(updated.Annotations, existing.Annotations) &&
		updated.Type == existing.Type &&
		reflect.DeepEqual(updated.OwnerReferences, existing.OwnerReferences) {
		return nil
	}

	return r.Update(ctx, updated)
}

func (r *ECRAuthReconciler) setCondition(
	ctx context.Context,
	auth *ecrv1alpha1.ECRAuth,
	status metav1.ConditionStatus,
	reason string,
	message string,
	lastSuccess *metav1.Time,
) error {
	updated := auth.DeepCopy()
	meta.SetStatusCondition(&updated.Status.Conditions, metav1.Condition{
		Type:               conditionTypeReady,
		Status:             status,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: updated.Generation,
	})
	updated.Status.ObservedGeneration = updated.Generation
	updated.Status.ManagedSecretName = updated.Spec.SecretName
	if lastSuccess != nil {
		updated.Status.LastSuccessfulRefreshTime = lastSuccess
	}

	if reflect.DeepEqual(updated.Status, auth.Status) {
		return nil
	}

	auth.Status = updated.Status
	if err := r.Status().Update(ctx, auth); err != nil {
		return fmt.Errorf("update status: %w", err)
	}
	return nil
}

func validateSpec(spec ecrv1alpha1.ECRAuthSpec) error {
	allErrs := field.ErrorList{}
	path := field.NewPath("spec")

	if strings.TrimSpace(spec.SecretName) == "" {
		allErrs = append(allErrs, field.Required(path.Child("secretName"), "secretName must be set"))
	} else {
		for _, msg := range validation.IsDNS1123Subdomain(spec.SecretName) {
			allErrs = append(allErrs, field.Invalid(path.Child("secretName"), spec.SecretName, msg))
		}
	}

	if len(spec.Registries) == 0 {
		allErrs = append(allErrs, field.Required(path.Child("registries"), "registries must contain at least one registry endpoint"))
	}

	seenRegistries := map[string]struct{}{}
	for i, rawRegistry := range spec.Registries {
		parsed, err := parseECRRegistry(rawRegistry)
		if err != nil {
			allErrs = append(allErrs, field.Invalid(path.Child("registries").Index(i), rawRegistry, err.Error()))
			continue
		}
		key := registryKey(parsed.AccountID, parsed.Region)
		if _, exists := seenRegistries[key]; exists {
			allErrs = append(allErrs, field.Duplicate(path.Child("registries").Index(i), rawRegistry))
			continue
		}
		seenRegistries[key] = struct{}{}
	}

	if spec.RefreshInterval != nil && spec.RefreshInterval.Duration <= 0 {
		allErrs = append(allErrs, field.Invalid(path.Child("refreshInterval"), spec.RefreshInterval.Duration.String(), "must be greater than zero"))
	}

	if len(allErrs) > 0 {
		return allErrs.ToAggregate()
	}
	return nil
}

func resolveRefreshInterval(interval *metav1.Duration) time.Duration {
	if interval == nil || interval.Duration <= 0 {
		return defaultRefreshInterval
	}
	return interval.Duration
}

func setManagedSecretMetadata(secret *corev1.Secret, auth *ecrv1alpha1.ECRAuth) {
	if secret.Labels == nil {
		secret.Labels = map[string]string{}
	}
	if secret.Annotations == nil {
		secret.Annotations = map[string]string{}
	}
	secret.Labels[managedByLabelKey] = managedByLabelValue
	secret.Annotations[ownerUIDAnnotation] = string(auth.UID)
}

func isManagedSecret(secret *corev1.Secret) bool {
	return secret.Labels != nil && secret.Labels[managedByLabelKey] == managedByLabelValue
}

func secretOwnedBy(secret *corev1.Secret, auth *ecrv1alpha1.ECRAuth) bool {
	if secret.Annotations != nil {
		if ownerUID, ok := secret.Annotations[ownerUIDAnnotation]; ok && ownerUID != "" {
			return ownerUID == string(auth.UID)
		}
	}
	return metav1.IsControlledBy(secret, auth)
}

func buildDockerConfigJSON(tokens []ECRAuthorizationToken) ([]byte, error) {
	if len(tokens) == 0 {
		return nil, errors.New("at least one authorization token is required")
	}

	type authEntry struct {
		Auth     string `json:"auth"`
		Username string `json:"username"`
		Password string `json:"password"`
	}
	type dockerConfig struct {
		Auths map[string]authEntry `json:"auths"`
	}

	payload := dockerConfig{Auths: map[string]authEntry{}}
	for i := range tokens {
		registry := strings.TrimSpace(tokens[i].ProxyEndpoint)
		username := strings.TrimSpace(tokens[i].Username)
		password := tokens[i].Password
		if registry == "" {
			return nil, fmt.Errorf("token %d: registry endpoint must be set", i)
		}
		if username == "" {
			return nil, fmt.Errorf("token %d: username must be set", i)
		}
		if password == "" {
			return nil, fmt.Errorf("token %d: password must be set", i)
		}
		if _, exists := payload.Auths[registry]; exists {
			return nil, fmt.Errorf("duplicate registry endpoint in tokens: %s", registry)
		}

		encodedAuth := base64.StdEncoding.EncodeToString([]byte(username + ":" + password))
		payload.Auths[registry] = authEntry{
			Auth:     encodedAuth,
			Username: username,
			Password: password,
		}
	}

	out, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal docker config: %w", err)
	}
	return out, nil
}

func decodeAuthorizationToken(encodedToken string) (string, string, error) {
	raw, err := base64.StdEncoding.DecodeString(encodedToken)
	if err != nil {
		return "", "", fmt.Errorf("decode base64 token: %w", err)
	}
	parts := strings.SplitN(string(raw), ":", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", errors.New("authorization token has invalid format")
	}
	return parts[0], parts[1], nil
}

func minDuration(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}
