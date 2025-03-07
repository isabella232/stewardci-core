package tenantctl

import (
	"context"
	"fmt"
	"testing"
	"time"

	stewardv1alpha1 "github.com/SAP/stewardci-core/pkg/apis/steward/v1alpha1"
	k8s "github.com/SAP/stewardci-core/pkg/k8s"
	k8sfake "github.com/SAP/stewardci-core/pkg/k8s/fake"
	k8smocks "github.com/SAP/stewardci-core/pkg/k8s/mocks"
	spew "github.com/davecgh/go-spew/spew"
	gomock "github.com/golang/mock/gomock"
	errors "github.com/pkg/errors"
	assert "gotest.tools/assert"
	is "gotest.tools/assert/cmp"
	rbacv1 "k8s.io/api/rbac/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	knativeapis "knative.dev/pkg/apis"
)

func Test_Controller_syncHandler_DoesNotingIfTenantNotFound(t *testing.T) {
	// SETUP
	cf := k8sfake.NewClientFactory( /* no objects exist */ )
	ctl := NewController(cf, ControllerOpts{})
	ctl.fetcher = k8s.NewClientBasedTenantFetcher(cf)

	// EXERCISE
	resultErr := ctl.syncHandler("nonexistentNamespace1/nonexistentTenant1")

	// VERIFY
	assert.NilError(t, resultErr)

	// K8s API actions
	{
		actions := cf.KubernetesClientset().Actions()
		assert.Assert(t, len(actions) == 0, spew.Sdump(actions))
	}
	// Steward API actions
	{
		actions := cf.StewardClientset().Actions()
		assert.Assert(t, len(actions) == 1, spew.Sdump(actions))
		action := actions[0]
		assert.Equal(t, "get", action.GetVerb(), spew.Sdump(action))
	}
}

func Test_Controller_syncHandler_FailsIfTenantFetchFails(t *testing.T) {
	// SETUP
	mockCtl := gomock.NewController(t)
	defer mockCtl.Finish()

	cf := k8sfake.NewClientFactory( /* no objects exist */ )

	fetcher := k8smocks.NewMockTenantFetcher(mockCtl)
	fetcherErr := errors.New("fetcher error")
	fetcher.EXPECT().ByKey(gomock.Not(gomock.Nil()), gomock.Any()).Return(nil, fetcherErr).Times(1)

	ctl := NewController(cf, ControllerOpts{})
	ctl.fetcher = fetcher

	// EXERCISE
	resultErr := ctl.syncHandler("namespace1/tenant1")

	// VERIFY
	assert.Equal(t, fetcherErr, resultErr)
}

func Test_Controller_syncHandler_FailsIfClientConfigIsInvalid(t *testing.T) {
	// SETUP
	const (
		clientNSName   = "client1"
		tenantID       = "tenant1"
		tenantNSPrefix = "prefix1"
		tenantRoleName = "tenantClusterRole1"
	)

	cf := k8sfake.NewClientFactory(
		// the client namespace
		k8sfake.Namespace(clientNSName), // annotations left out because not needed
		// the tenant
		k8sfake.Tenant(tenantID, clientNSName),
	)
	ctl := NewController(cf, ControllerOpts{})
	ctl.fetcher = k8s.NewClientBasedTenantFetcher(cf)

	injectedError := errors.New("ERR1")
	ctl.testing = &controllerTesting{
		getClientConfigStub: func(k8s.ClientFactory, string) (clientConfig, error) {
			return nil, injectedError
		},
	}

	// EXERCISE
	resultErr := ctl.syncHandler(makeTenantKey(clientNSName, tenantID))

	// VERIFY
	assert.Assert(t, injectedError == resultErr)
	assertThatExactlyTheseNamespacesExist(t, cf,
		clientNSName,
	)
	assertThatExactlyTheseTenantsExistInNamespace(t, cf, clientNSName,
		tenantID,
	)
}

func Test_Controller_syncHandler_AddsFinalizer(t *testing.T) {
	// SETUP
	const (
		clientNSName   = "client1"
		tenantID       = "tenant1"
		tenantNSPrefix = "prefix1"
		tenantRoleName = "tenantClusterRole1"
	)

	ctx := context.Background()
	cf := k8sfake.NewClientFactory(
		// the client namespace
		k8sfake.NamespaceWithAnnotations(clientNSName, map[string]string{
			stewardv1alpha1.AnnotationTenantNamespacePrefix: tenantNSPrefix,
			stewardv1alpha1.AnnotationTenantRole:            tenantRoleName,
		}),
		// the tenant
		k8sfake.Tenant(tenantID, clientNSName),
	)
	ctl := NewController(cf, ControllerOpts{})
	ctl.fetcher = k8s.NewClientBasedTenantFetcher(cf)
	// ensure that there are no finalizers
	{
		tenant, err := cf.StewardV1alpha1().Tenants(clientNSName).Get(ctx, tenantID, metav1.GetOptions{})
		assert.NilError(t, err)
		assertThatExactlyTheseFinalizersExist(t, &tenant.ObjectMeta /*none*/)
	}

	// EXERCISE
	resultErr := ctl.syncHandler(makeTenantKey(clientNSName, tenantID))

	// VERIFY
	assert.NilError(t, resultErr)
	{
		tenant, err := cf.StewardV1alpha1().Tenants(clientNSName).Get(ctx, tenantID, metav1.GetOptions{})
		assert.NilError(t, err)
		assertThatExactlyTheseFinalizersExist(t, &tenant.ObjectMeta,
			k8s.FinalizerName,
		)
	}
}

func Test_Controller_syncHandler_UninitializedTenant_GoodCase(t *testing.T) {
	// SETUP
	const (
		clientNSName   = "client1"
		tenantNSPrefix = "prefix1"
		tenantID       = "tenant1"
		tenantRoleName = "tenantClusterRole1"
	)

	ctx := context.Background()
	cf := k8sfake.NewClientFactory(
		// the client namespace
		k8sfake.NamespaceWithAnnotations(clientNSName, map[string]string{
			stewardv1alpha1.AnnotationTenantNamespacePrefix: tenantNSPrefix,
			stewardv1alpha1.AnnotationTenantRole:            tenantRoleName,
		}),
		// the tenant
		k8sfake.Tenant(tenantID, clientNSName),
	)
	ctl := NewController(cf, ControllerOpts{})
	ctl.fetcher = k8s.NewClientBasedTenantFetcher(cf)

	// EXERCISE
	resultErr := ctl.syncHandler(makeTenantKey(clientNSName, tenantID))

	// VERIFY
	assert.NilError(t, resultErr)
	tenant, err := cf.StewardV1alpha1().Tenants(clientNSName).Get(ctx, tenantID, metav1.GetOptions{})
	assert.NilError(t, err)

	// tenant
	{
		dump := fmt.Sprintf("\n\n%v", spew.Sdump(tenant))
		{
			readyCond := tenant.Status.GetCondition(knativeapis.ConditionReady)
			assert.Assert(t, readyCond.IsTrue(), dump)
		}
		{
			nsNamePattern := fmt.Sprintf(`^\Q%s\E-\Q%s\E-[0-9a-z]+$`, tenantNSPrefix, tenantID)
			assert.Assert(t, is.Regexp(nsNamePattern, tenant.Status.TenantNamespaceName), dump)
		}
	}

	// tenant namespace
	{
		namespace, err := cf.CoreV1().Namespaces().Get(ctx, tenant.Status.TenantNamespaceName, metav1.GetOptions{})
		assert.NilError(t, err)

		_, labelExists := namespace.GetLabels()[stewardv1alpha1.LabelSystemManaged]
		assert.Assert(t, !labelExists)
	}

	// RoleBinding in tenant namespace
	{
		roleBindingList, err := cf.RbacV1().RoleBindings(tenant.Status.TenantNamespaceName).
			List(ctx, metav1.ListOptions{LabelSelector: stewardv1alpha1.LabelSystemManaged})
		assert.NilError(t, err)
		assert.Assert(t, len(roleBindingList.Items) == 1)
		roleBinding := roleBindingList.Items[0]

		_, labelExists := roleBinding.GetLabels()[stewardv1alpha1.LabelSystemManaged]
		assert.Assert(t, labelExists)

		expectedRoleRef := rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     tenantRoleName,
		}
		assert.DeepEqual(t, expectedRoleRef, roleBinding.RoleRef)

		expectedSubjects := []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Namespace: tenant.Status.TenantNamespaceName,
				Name:      "default",
			},
			{
				Kind:      "ServiceAccount",
				Namespace: clientNSName,
				Name:      "default",
			},
		}
		assert.DeepEqual(t, expectedSubjects, roleBinding.Subjects)
	}
}

func Test_Controller_syncHandler_UninitializedTenant_FailsOnNamespaceClash(t *testing.T) {
	// SETUP
	const (
		clientNSName   = "client1"
		tenantNSPrefix = "prefix1"
		tenantID       = "tenant1"
		tenantRoleName = "tenantClusterRole1"
	)

	ctx := context.Background()
	clashingNamespaceName := fmt.Sprintf("%s-%s", tenantNSPrefix, tenantID)
	cf := k8sfake.NewClientFactory(
		// the client namespace
		k8sfake.NamespaceWithAnnotations(clientNSName, map[string]string{
			stewardv1alpha1.AnnotationTenantNamespacePrefix:       tenantNSPrefix,
			stewardv1alpha1.AnnotationTenantNamespaceSuffixLength: "0",
			stewardv1alpha1.AnnotationTenantRole:                  tenantRoleName,
		}),
		// the tenant
		k8sfake.Tenant(tenantID, clientNSName),
		// a namespace with same name as will be used for tenant namespace
		k8sfake.Namespace(clashingNamespaceName),
	)
	ctl := NewController(cf, ControllerOpts{})
	ctl.fetcher = k8s.NewClientBasedTenantFetcher(cf)

	// EXERCISE
	resultErr := ctl.syncHandler(makeTenantKey(clientNSName, tenantID))

	// VERIFY
	assert.Assert(t, resultErr != nil)
	assert.Assert(t, is.Regexp(
		`^failed to create new tenant namespace: .`,
		resultErr.Error(),
	))

	tenant, err := cf.StewardV1alpha1().Tenants(clientNSName).Get(ctx, tenantID, metav1.GetOptions{})
	assert.NilError(t, err)

	// tenant
	{
		dump := fmt.Sprintf("\n\n%v", spew.Sdump(tenant))
		{
			readyCond := tenant.Status.GetCondition(knativeapis.ConditionReady)
			assert.Assert(t, readyCond.IsFalse(), dump)
			assert.Equal(t, stewardv1alpha1.StatusReasonFailed, readyCond.Reason, dump)
			assert.Equal(t, "Failed to create a new tenant namespace.", readyCond.Message, dump)
		}
		assert.Equal(t, "", tenant.Status.TenantNamespaceName, dump)
	}

	assertThatExactlyTheseNamespacesExist(t, cf,
		clientNSName,
		clashingNamespaceName,
	)

	// RoleBinding in tenant namespace NOT created
	{
		_, err := cf.RbacV1().RoleBindings(tenant.Status.TenantNamespaceName).
			Get(ctx, tenantNamespaceRoleBindingNamePrefix, metav1.GetOptions{})
		assert.Assert(t, kerrors.IsNotFound(err))
	}
}

func Test_Controller_syncHandler_UninitializedTenant_FailsOnErrorWhenSyncingRoleBinding(t *testing.T) {
	// SETUP
	const (
		clientNSName   = "client1"
		tenantNSPrefix = "prefix1"
		tenantID       = "tenant1"
		tenantRoleName = "tenantClusterRole1"
	)

	cf := k8sfake.NewClientFactory(
		// the client namespace
		k8sfake.NamespaceWithAnnotations(clientNSName, map[string]string{
			stewardv1alpha1.AnnotationTenantNamespacePrefix: tenantNSPrefix,
			stewardv1alpha1.AnnotationTenantRole:            tenantRoleName,
		}),
		// the tenant
		k8sfake.Tenant(tenantID, clientNSName),
	)
	ctl := NewController(cf, ControllerOpts{})
	ctl.fetcher = k8s.NewClientBasedTenantFetcher(cf)

	injectedError := errors.New("ERR1")
	ctl.testing = &controllerTesting{
		reconcileTenantRoleBindingStub: func(*stewardv1alpha1.Tenant, string, clientConfig) (bool, error) {
			return false, injectedError
		},
	}

	// EXERCISE
	resultErr := ctl.syncHandler(makeTenantKey(clientNSName, tenantID))

	// VERIFY
	assert.Assert(t, resultErr != nil)
	assert.Assert(t, injectedError == resultErr)

	ctx := context.Background()
	tenant, err := cf.StewardV1alpha1().Tenants(clientNSName).Get(ctx, tenantID, metav1.GetOptions{})
	assert.NilError(t, err)

	// tenant
	{
		dump := fmt.Sprintf("\n\n%v", spew.Sdump(tenant))
		{
			readyCond := tenant.Status.GetCondition(knativeapis.ConditionReady)
			assert.Assert(t, readyCond.IsFalse(), dump)
			assert.Equal(t, stewardv1alpha1.StatusReasonFailed, readyCond.Reason, dump)
			assert.Equal(t, "Failed to initialize a new tenant namespace because the RoleBinding could not be created.", readyCond.Message, dump)
		}
		assert.Equal(t, "", tenant.Status.TenantNamespaceName, dump)
	}

	assertThatExactlyTheseNamespacesExist(t, cf,
		clientNSName,
	)
}

func Test_Controller_syncHandler_InitializedTenant_AddsMissingRoleBinding(t *testing.T) {
	// SETUP
	const (
		clientNSName   = "client1"
		tenantNSPrefix = "prefix1"
		tenantID       = "tenant1"
		tenantRoleName = "tenantClusterRole1"

		tenantNSName = "somename1"
	)

	origTenant := k8sfake.Tenant(tenantID, clientNSName)
	origTenant.Status.TenantNamespaceName = tenantNSName
	// no ready condition set because not needed by the reconciler

	cf := k8sfake.NewClientFactory(
		// the client namespace
		k8sfake.NamespaceWithAnnotations(clientNSName, map[string]string{
			stewardv1alpha1.AnnotationTenantNamespacePrefix: tenantNSPrefix,
			stewardv1alpha1.AnnotationTenantRole:            tenantRoleName,
		}),
		// the tenant
		origTenant,
		// the tenant namespace
		k8sfake.Namespace(tenantNSName),
	)
	ctl := NewController(cf, ControllerOpts{})
	ctl.fetcher = k8s.NewClientBasedTenantFetcher(cf)

	// EXERCISE
	resultErr := ctl.syncHandler(makeTenantKey(clientNSName, tenantID))

	// VERIFY
	assert.NilError(t, resultErr)
	ctx := context.Background()
	tenant, err := cf.StewardV1alpha1().Tenants(clientNSName).Get(ctx, tenantID, metav1.GetOptions{})
	assert.NilError(t, err)

	// tenant
	{
		dump := fmt.Sprintf("\n\n%v", spew.Sdump(tenant))
		{
			readyCond := tenant.Status.GetCondition(knativeapis.ConditionReady)
			assert.Assert(t, readyCond.IsTrue(), dump)
		}
		assert.Equal(t, tenantNSName, tenant.Status.TenantNamespaceName, dump)
	}

	// tenant namespace
	assertThatExactlyTheseNamespacesExist(t, cf,
		clientNSName,
		tenantNSName,
	)

	// RoleBinding in tenant namespace
	{
		roleBindingList, err := cf.RbacV1().RoleBindings(tenant.Status.TenantNamespaceName).
			List(ctx, metav1.ListOptions{LabelSelector: stewardv1alpha1.LabelSystemManaged})
		assert.NilError(t, err)
		assert.Assert(t, len(roleBindingList.Items) == 1)
		roleBinding := roleBindingList.Items[0]

		_, labelExists := roleBinding.GetLabels()[stewardv1alpha1.LabelSystemManaged]
		assert.Assert(t, labelExists)

		expectedRoleRef := rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     tenantRoleName,
		}
		assert.DeepEqual(t, expectedRoleRef, roleBinding.RoleRef)

		expectedSubjects := []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Namespace: tenant.Status.TenantNamespaceName,
				Name:      "default",
			},
			{
				Kind:      "ServiceAccount",
				Namespace: clientNSName,
				Name:      "default",
			},
		}
		assert.DeepEqual(t, expectedSubjects, roleBinding.Subjects)
	}
}

func Test_Controller_syncHandler_InitializedTenant_FailsOnMissingNamespace(t *testing.T) {
	// SETUP
	const (
		clientNSName   = "client1"
		tenantNSPrefix = "prefix1"
		tenantID       = "tenant1"
		tenantRoleName = "tenantClusterRole1"
		tenantNSName   = "somename1"
	)

	origTenant := k8sfake.Tenant(tenantID, clientNSName)
	origTenant.Status.TenantNamespaceName = tenantNSName
	// no ready condition set because not needed by the reconciler

	cf := k8sfake.NewClientFactory(
		// the client namespace
		k8sfake.NamespaceWithAnnotations(clientNSName, map[string]string{
			stewardv1alpha1.AnnotationTenantNamespacePrefix: tenantNSPrefix,
			stewardv1alpha1.AnnotationTenantRole:            tenantRoleName,
		}),
		// the tenant
		origTenant,
		// no tenant namespace here,
	)
	ctl := NewController(cf, ControllerOpts{})
	ctl.fetcher = k8s.NewClientBasedTenantFetcher(cf)

	// EXERCISE
	resultErr := ctl.syncHandler(makeTenantKey(clientNSName, tenantID))

	// VERIFY
	assert.Assert(t, resultErr != nil)
	assert.Error(t, resultErr, fmt.Sprintf("tenant namespace \"%s\" does not exist anymore", tenantNSName))

	ctx := context.Background()
	tenant, err := cf.StewardV1alpha1().Tenants(clientNSName).Get(ctx, tenantID, metav1.GetOptions{})
	assert.NilError(t, err)

	// tenant
	{
		dump := fmt.Sprintf("\n\n%v", spew.Sdump(tenant))
		{
			readyCond := tenant.Status.GetCondition(knativeapis.ConditionReady)
			assert.Assert(t, readyCond.IsFalse(), dump)
			assert.Equal(t, stewardv1alpha1.StatusReasonDependentResourceState, readyCond.Reason, dump)
			assert.Equal(t,
				fmt.Sprintf(
					"The tenant namespace \"%s\" does not exist anymore."+
						" This issue must be analyzed and fixed by an operator.",
					tenantNSName,
				),
				readyCond.Message,
				dump,
			)
		}
		assert.Equal(t, tenantNSName, tenant.Status.TenantNamespaceName, dump)
	}

	assertThatExactlyTheseNamespacesExist(t, cf,
		clientNSName,
	)
}

func Test_Controller_syncHandler_InitializedTenant_FailsOnErrorWhenSyncingRoleBinding(t *testing.T) {
	// SETUP
	const (
		clientNSName   = "client1"
		tenantNSPrefix = "prefix1"
		tenantID       = "tenant1"
		tenantRoleName = "tenantClusterRole1"
		tenantNSName   = "somename1"
	)

	origTenant := k8sfake.Tenant(tenantID, clientNSName)
	origTenant.Status.TenantNamespaceName = tenantNSName
	// no ready condition set because not needed by the reconciler

	cf := k8sfake.NewClientFactory(
		// the client namespace
		k8sfake.NamespaceWithAnnotations(clientNSName, map[string]string{
			stewardv1alpha1.AnnotationTenantNamespacePrefix: tenantNSPrefix,
			stewardv1alpha1.AnnotationTenantRole:            tenantRoleName,
		}),
		// the tenant
		origTenant,
		// the tenant namespace
		k8sfake.Namespace(tenantNSName),
	)
	ctl := NewController(cf, ControllerOpts{})
	ctl.fetcher = k8s.NewClientBasedTenantFetcher(cf)

	injectedError := errors.New("ERR1")
	ctl.testing = &controllerTesting{
		reconcileTenantRoleBindingStub: func(*stewardv1alpha1.Tenant, string, clientConfig) (bool, error) {
			return true, injectedError
		},
	}

	// EXERCISE
	resultErr := ctl.syncHandler(makeTenantKey(clientNSName, tenantID))

	// VERIFY
	assert.Assert(t, resultErr != nil)
	assert.Assert(t, injectedError == resultErr)

	ctx := context.Background()
	tenant, err := cf.StewardV1alpha1().Tenants(clientNSName).Get(ctx, tenantID, metav1.GetOptions{})
	assert.NilError(t, err)

	// status
	{
		dump := fmt.Sprintf("\n\n%v", spew.Sdump(tenant.Status))

		readyCond := tenant.Status.GetCondition(knativeapis.ConditionReady)
		assert.Assert(t, readyCond.IsFalse(), dump)
		assert.Equal(t, stewardv1alpha1.StatusReasonDependentResourceState, readyCond.Reason, dump)
		assert.Equal(t,
			fmt.Sprintf(
				"The RoleBinding in tenant namespace \"%s\" is outdated but could not be updated.",
				tenantNSName,
			),
			readyCond.Message,
			dump,
		)

		assert.Equal(t, tenantNSName, tenant.Status.TenantNamespaceName, dump)
	}

	assertThatExactlyTheseNamespacesExist(t, cf,
		clientNSName,
		tenantNSName,
	)
}

func Test_Controller_syncHandler_CleanupOnDelete_IfFinalizerIsSet(t *testing.T) {
	// SETUP
	const (
		clientNSName   = "client1"
		tenantNSPrefix = "prefix1"
		tenantID       = "tenant1"
		tenantRoleName = "tenantClusterRole1"
	)

	ctx := context.Background()
	cf := k8sfake.NewClientFactory(
		// the client namespace
		k8sfake.NamespaceWithAnnotations(clientNSName, map[string]string{
			stewardv1alpha1.AnnotationTenantNamespacePrefix: tenantNSPrefix,
			stewardv1alpha1.AnnotationTenantRole:            tenantRoleName,
		}),
		// the tenant
		k8sfake.Tenant(tenantID, clientNSName),
	)
	ctl := NewController(cf, ControllerOpts{})
	ctl.fetcher = k8s.NewClientBasedTenantFetcher(cf)
	tenantKey := makeTenantKey(clientNSName, tenantID)
	tenantsIfc := cf.StewardV1alpha1().Tenants(clientNSName)
	var tenantNSName string

	// initialize tenant
	{
		err := ctl.syncHandler(tenantKey)
		assert.NilError(t, err)

		initializedTenant, err := tenantsIfc.Get(ctx, tenantID, metav1.GetOptions{})
		assert.NilError(t, err)
		tenantNSName = initializedTenant.Status.TenantNamespaceName
	}

	assert.Assert(t, tenantNSName != "")
	assertThatExactlyTheseNamespacesExist(t, cf,
		clientNSName,
		tenantNSName, // tenant namespace created
	)

	// mark tenant as deleted
	{
		// Fake client deletes immediately -> set deletion timestamp
		tenant, err := tenantsIfc.Get(ctx, tenantID, metav1.GetOptions{})
		assert.NilError(t, err)
		tenant.SetDeletionTimestamp(&metav1.Time{Time: time.Now()})
		_, err = tenantsIfc.Update(ctx, tenant, metav1.UpdateOptions{})
		assert.NilError(t, err)
	}

	// tenant still exists due to finalizer
	assertThatExactlyTheseTenantsExistInNamespace(t, cf, clientNSName,
		tenantID,
	)

	// EXERCISE
	resultErr := ctl.syncHandler(tenantKey)

	// VERIFY
	assert.NilError(t, resultErr)
	assertThatExactlyTheseNamespacesExist(t, cf,
		clientNSName,
		// tenant namespace removed
	)
	assertThatExactlyTheseTenantsExistInNamespace(t, cf, clientNSName /*none*/)
}

func Test_Controller_syncHandler_CleanupOnDelete_SkippedIfFinalizerIsNotSet(t *testing.T) {
	// SETUP
	const (
		clientNSName   = "client1"
		tenantNSPrefix = "prefix1"
		tenantID       = "tenant1"
		tenantRoleName = "tenantClusterRole1"
	)

	ctx := context.Background()
	cf := k8sfake.NewClientFactory(
		// the client namespace
		k8sfake.NamespaceWithAnnotations(clientNSName, map[string]string{
			stewardv1alpha1.AnnotationTenantNamespacePrefix: tenantNSPrefix,
			stewardv1alpha1.AnnotationTenantRole:            tenantRoleName,
		}),
		// the tenant
		k8sfake.Tenant(tenantID, clientNSName),
	)
	ctl := NewController(cf, ControllerOpts{})
	ctl.fetcher = k8s.NewClientBasedTenantFetcher(cf)
	tenantKey := makeTenantKey(clientNSName, tenantID)
	tenantsIfc := cf.StewardV1alpha1().Tenants(clientNSName)
	var tenantNSName string

	// initialize tenant
	{
		err := ctl.syncHandler(tenantKey)
		assert.NilError(t, err)

		initializedTenant, err := tenantsIfc.Get(ctx, tenantID, metav1.GetOptions{})
		assert.NilError(t, err)
		tenantNSName = initializedTenant.Status.TenantNamespaceName
	}

	assert.Assert(t, tenantNSName != "")
	assertThatExactlyTheseNamespacesExist(t, cf,
		clientNSName,
		tenantNSName, // tenant namespace created
	)

	// mark tenant as deleted
	{
		// Fake client deletes immediately -> set deletion timestamp
		tenant, err := tenantsIfc.Get(ctx, tenantID, metav1.GetOptions{})
		assert.NilError(t, err)
		tenant.SetDeletionTimestamp(&metav1.Time{Time: time.Now()})
		tenant.SetFinalizers([]string{"not-our-finalizer"})
		_, err = tenantsIfc.Update(ctx, tenant, metav1.UpdateOptions{})
		assert.NilError(t, err)
	}

	// EXERCISE
	resultErr := ctl.syncHandler(tenantKey)

	// VERIFY
	assert.NilError(t, resultErr)
	assertThatExactlyTheseNamespacesExist(t, cf,
		clientNSName,
		tenantNSName, // tenant namespace NOT removed
	)
	assertThatExactlyTheseTenantsExistInNamespace(t, cf, clientNSName,
		tenantID, // due to other finalizer
	)
	tenant, err := tenantsIfc.Get(ctx, tenantID, metav1.GetOptions{})
	assert.NilError(t, err)
	assert.Assert(t, !tenant.GetDeletionTimestamp().IsZero())
	assertThatExactlyTheseFinalizersExist(t, &tenant.ObjectMeta, "not-our-finalizer")
}

func Test_Controller_syncHandler_CleanupOnDelete_IfNamespaceDoesNotExistAnymore(t *testing.T) {
	// SETUP
	const (
		clientNSName   = "client1"
		tenantNSPrefix = "prefix1"
		tenantID       = "tenant1"
		tenantRoleName = "tenantClusterRole1"
	)

	ctx := context.Background()
	cf := k8sfake.NewClientFactory(
		// the client namespace
		k8sfake.NamespaceWithAnnotations(clientNSName, map[string]string{
			stewardv1alpha1.AnnotationTenantNamespacePrefix: tenantNSPrefix,
			stewardv1alpha1.AnnotationTenantRole:            tenantRoleName,
		}),
		// the tenant
		k8sfake.Tenant(tenantID, clientNSName),
	)
	ctl := NewController(cf, ControllerOpts{})
	ctl.fetcher = k8s.NewClientBasedTenantFetcher(cf)
	tenantKey := makeTenantKey(clientNSName, tenantID)
	tenantsIfc := cf.StewardV1alpha1().Tenants(clientNSName)
	var tenantNSName string

	// initialize tenant
	{
		err := ctl.syncHandler(tenantKey)
		assert.NilError(t, err)

		initializedTenant, err := tenantsIfc.Get(ctx, tenantID, metav1.GetOptions{})
		assert.NilError(t, err)
		tenantNSName = initializedTenant.Status.TenantNamespaceName
	}

	assert.Assert(t, tenantNSName != "")
	assertThatExactlyTheseNamespacesExist(t, cf,
		clientNSName,
		tenantNSName, // tenant namespace created
	)

	// delete tenant namespace
	{
		err := cf.CoreV1().Namespaces().Delete(ctx, tenantNSName, metav1.DeleteOptions{})
		assert.NilError(t, err)
	}

	assertThatExactlyTheseNamespacesExist(t, cf,
		clientNSName,
		// tenant namespace deleted
	)

	// mark tenant as deleted
	{
		// Fake client deletes immediately -> set deletion timestamp
		tenant, err := tenantsIfc.Get(ctx, tenantID, metav1.GetOptions{})
		assert.NilError(t, err)
		tenant.SetDeletionTimestamp(&metav1.Time{Time: time.Now()})
		_, err = tenantsIfc.Update(ctx, tenant, metav1.UpdateOptions{})
		assert.NilError(t, err)
	}

	// tenant still exists due to finalizer
	assertThatExactlyTheseTenantsExistInNamespace(t, cf, clientNSName,
		tenantID,
	)

	// EXERCISE
	resultErr := ctl.syncHandler(tenantKey)

	// VERIFY
	assert.NilError(t, resultErr)
	assertThatExactlyTheseTenantsExistInNamespace(t, cf, clientNSName /*none*/)
}

func Test_Controller_syncHandler_CleanupOnStatusUpdateFailure(t *testing.T) {
	// SETUP
	const (
		clientNSName   = "client1"
		tenantNSPrefix = "prefix1"
		tenantID       = "tenant1"
		tenantRoleName = "tenantClusterRole1"
	)

	cf := k8sfake.NewClientFactory(
		// the client namespace
		k8sfake.NamespaceWithAnnotations(clientNSName, map[string]string{
			stewardv1alpha1.AnnotationTenantNamespacePrefix: tenantNSPrefix,
			stewardv1alpha1.AnnotationTenantRole:            tenantRoleName,
		}),
		// the tenant
		k8sfake.Tenant(tenantID, clientNSName),
	)
	ctl := NewController(cf, ControllerOpts{})
	ctl.fetcher = k8s.NewClientBasedTenantFetcher(cf)

	injectedError := errors.New("ERR1")
	ctl.testing = &controllerTesting{
		updateStatusStub: func(tenant *stewardv1alpha1.Tenant) (*stewardv1alpha1.Tenant, error) {
			assert.Assert(t, tenant.Status.TenantNamespaceName != "", spew.Sdump(tenant.Status))
			return tenant, injectedError
		},
	}

	// EXERCISE
	resultErr := ctl.syncHandler(makeTenantKey(clientNSName, tenantID))

	// VERIFY
	assert.Assert(t, injectedError == resultErr)

	assertThatExactlyTheseNamespacesExist(t, cf,
		clientNSName,
		/* no tenant namespace */
	)
}

func Test_Controller_reconcileTenantRoleBinding_FailsOnErrorIn_listManagedRoleBindings(t *testing.T) {
	// SETUP
	const (
		clientNSName   = "client1"
		tenantNSName   = "tenantNS1"
		tenantID       = "tenant1"
		tenantRoleName = "tenantClusterRole1"
	)

	ctx := context.Background()
	tenant := k8sfake.Tenant(tenantID, clientNSName)
	config := &clientConfigImpl{
		tenantRoleName: tenantRoleName,
	}

	injectedError := errors.Errorf("injected error 1")

	examinee := &Controller{
		testing: &controllerTesting{
			listManagedRoleBindingsStub: func(string) (*rbacv1.RoleBindingList, error) {
				return nil, injectedError
			},
		},
	}

	// EXERCISE
	resultUpdateNeeded, resultErr := examinee.reconcileTenantRoleBinding(ctx, tenant, tenantNSName, config)

	// VERIFY
	assert.Error(t, resultErr, fmt.Sprintf(
		"failed to reconcile the RoleBinding in tenant namespace \"%s\": injected error 1",
		tenantNSName,
	))
	assert.Assert(t, errors.Cause(resultErr) == injectedError)
	assert.Assert(t, resultUpdateNeeded == false)
}

func Test_Controller_reconcileTenantRoleBinding_FailsOnErrorIn_createRoleBinding(t *testing.T) {
	// SETUP
	const (
		clientNSName   = "client1"
		tenantNSName   = "tenantNS1"
		tenantID       = "tenant1"
		tenantRoleName = "tenantClusterRole1"
	)

	ctx := context.Background()
	tenant := k8sfake.Tenant(tenantID, clientNSName)
	config := &clientConfigImpl{
		tenantRoleName: tenantRoleName,
	}

	injectedError := errors.Errorf("injected error 1")

	examinee := &Controller{
		testing: &controllerTesting{
			listManagedRoleBindingsStub: func(string) (*rbacv1.RoleBindingList, error) {
				return &rbacv1.RoleBindingList{}, nil
			},
			createRoleBindingStub: func(*rbacv1.RoleBinding) (*rbacv1.RoleBinding, error) {
				return nil, injectedError
			},
		},
	}

	// EXERCISE
	resultUpdateNeeded, resultErr := examinee.reconcileTenantRoleBinding(ctx, tenant, tenantNSName, config)

	// VERIFY
	assert.Error(t, resultErr, fmt.Sprintf(
		"failed to reconcile the RoleBinding in tenant namespace \"%s\": injected error 1",
		tenantNSName,
	))
	assert.Assert(t, errors.Cause(resultErr) == injectedError)
	assert.Assert(t, resultUpdateNeeded == true)
}

func Test_Controller_listManagedRoleBindings_GoodCase_WithLabelFilter(t *testing.T) {
	// SETUP
	const (
		nsName = "namespace1"
	)

	ctx := context.Background()
	newManagedRoleBinding := func(name string, labelValue string) *rbacv1.RoleBinding {
		return &rbacv1.RoleBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: nsName,
				Labels: map[string]string{
					stewardv1alpha1.LabelSystemManaged: labelValue, // SUT's selector should not depend on that value
				},
			},
		}
	}
	newUnmanagedRoleBinding := func(name string) *rbacv1.RoleBinding {
		return &rbacv1.RoleBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: nsName,
			},
		}
	}

	cf := k8sfake.NewClientFactory(
		newManagedRoleBinding("roleBinding1", ""),
		newUnmanagedRoleBinding("roleBinding2"),
		newManagedRoleBinding("roleBinding3", "dfkghsdfasdfk"),
		newUnmanagedRoleBinding("roleBinding4"),
		newManagedRoleBinding("roleBinding5", "false"),
	)

	examinee := &Controller{factory: cf}

	// EXERCISE
	resultList, resultErr := examinee.listManagedRoleBindings(ctx, nsName)

	// VERIFY
	assert.NilError(t, resultErr)
	assert.Assert(t, resultList != nil)

	{
		itemNames := make([]string, len(resultList.Items))
		for i, item := range resultList.Items {
			itemNames[i] = item.GetName()
		}
		assert.DeepEqual(t,
			[]string{
				"roleBinding1",
				"roleBinding3",
				"roleBinding5",
			},
			itemNames,
		)
	}
}

func Test_Controller_listManagedRoleBindings_FailureCase(t *testing.T) {
	// SETUP
	const (
		nsName = "namespace1"
	)

	ctx := context.Background()
	cf := k8sfake.NewClientFactory()
	injectedError := errors.Errorf("injected error 1")
	cf.KubernetesClientset().PrependReactor("list", "rolebindings", k8sfake.NewErrorReactor(injectedError))

	examinee := &Controller{factory: cf}

	// EXERCISE
	resultList, resultErr := examinee.listManagedRoleBindings(ctx, nsName)

	// VERIFY
	assert.Assert(t, resultErr != nil)
	assert.Error(t, resultErr, fmt.Sprintf(
		"failed to get all managed RoleBindings from namespace \"%s\": injected error 1",
		nsName,
	))
	assert.Assert(t, errors.Cause(resultErr) == injectedError)
	assert.Assert(t, resultList == nil)
}

//Test for ERROR: Failed to update status of tenant '4e93d9d5-276e-47ca-a570-b3a763aaef3e' in namespace 'stu':
//         Operation cannot be fulfilled on tenants.steward.sap.com "4e93d9d5-276e-47ca-a570-b3a763aaef3e":
//         the object has been modified; please apply your changes to the latest version and try again
func Test_Controller_updateStatus_ConcurrentModification(t *testing.T) {
	t.Skip("does not work with fake clients as those do not manage UID, resource version, generation etc.")

	// SETUP
	const (
		clientNSName   = "client1"
		tenantID       = "tenant1"
		tenantRoleName = "tenantClusterRole1"
	)

	ctx := context.Background()
	cf := k8sfake.NewClientFactory(
		// the client namespace
		k8sfake.NamespaceWithAnnotations(clientNSName, map[string]string{
			stewardv1alpha1.AnnotationTenantNamespacePrefix: "prefix1",
			stewardv1alpha1.AnnotationTenantRole:            tenantRoleName,
		}),
		// the tenant
		k8sfake.Tenant(tenantID, clientNSName),
	)

	// EXERCISE + VERIFY
	stopCh, controller := startController(t, cf)
	defer stopController(t, stopCh)

	tenant, err := cf.StewardV1alpha1().Tenants(clientNSName).Get(ctx, tenantID, metav1.GetOptions{})
	assert.NilError(t, err)

	// first update
	{
		cond := tenant.Status.GetCondition(knativeapis.ConditionReady)
		cond.Message = "update 1"
		tenant.Status.SetCondition(cond)
		_, err = controller.updateStatus(ctx, tenant)
		assert.NilError(t, err)
	}

	// second update based on the same revision as the first one
	{
		//TODO This update should fail but doesn't with the fakes
		cond := tenant.Status.GetCondition(knativeapis.ConditionReady)
		cond.Message = "update 2"
		tenant.Status.SetCondition(cond)
		if _, err := controller.updateStatus(ctx, tenant); err == nil {
			t.Fatalf("second update succeeded but should have failed")
		}
	}
}

func Test_Controller_FullWorkflow(t *testing.T) {
	// SETUP
	const (
		clientNSName   = "client1"
		tenantID       = "tenant1"
		tenantNSPrefix = "prefix1"
		tenantRoleName = "tenantClusterRole1"
		tenantNSName   = tenantNSPrefix + "-" + tenantID
	)

	cf := k8sfake.NewClientFactory(
		// the client namespace
		k8sfake.NamespaceWithAnnotations(clientNSName, map[string]string{
			stewardv1alpha1.AnnotationTenantNamespacePrefix:       tenantNSPrefix,
			stewardv1alpha1.AnnotationTenantRole:                  tenantRoleName,
			stewardv1alpha1.AnnotationTenantNamespaceSuffixLength: "0",
		}),
	)

	// EXERCISE
	stopCh, controller := startController(t, cf)
	defer stopController(t, stopCh)

	// VERIFY
	tenantsIfc := cf.StewardV1alpha1().Tenants(clientNSName)
	ctx := context.Background()

	assertThatExactlyTheseNamespacesExist(t, cf,
		clientNSName,
	)
	assertThatExactlyTheseTenantsExistInNamespace(t, cf, clientNSName /* none */)

	t.Log("Stage: Create tenant")
	{
		syncCount := controller.getSyncCount()

		_, err := tenantsIfc.Create(ctx, k8sfake.Tenant(tenantID, clientNSName), metav1.CreateOptions{})
		assert.NilError(t, err)

		waitForNextSync(t, controller, syncCount)

		assertThatExactlyTheseNamespacesExist(t, cf,
			clientNSName,
			tenantNSName,
		)
		assertThatExactlyTheseTenantsExistInNamespace(t, cf, clientNSName,
			tenantID,
		)

		tenant, err := tenantsIfc.Get(ctx, tenantID, metav1.GetOptions{})
		assert.NilError(t, err)
		dump := fmt.Sprintf("\n\n%v", spew.Sdump(tenant))
		{
			readyCond := tenant.Status.GetCondition(knativeapis.ConditionReady)
			assert.Assert(t, readyCond.IsTrue(), dump)
		}
		assert.Equal(t, tenantNSName, tenant.Status.TenantNamespaceName)

		// TODO check role binding

		assert.Equal(t, 1, len(tenant.GetFinalizers()))
	}

	t.Log("Stage: Delete tenant")
	{
		syncCount := controller.getSyncCount()

		tenant, err := tenantsIfc.Get(ctx, tenantID, metav1.GetOptions{})
		assert.NilError(t, err)

		// Fake client deletes immediately -> set deletion timestamp
		tenant.SetDeletionTimestamp(&metav1.Time{Time: time.Now()})
		_, err = tenantsIfc.Update(ctx, tenant, metav1.UpdateOptions{})
		assert.NilError(t, err)

		waitForNextSync(t, controller, syncCount)

		tenant, err = tenantsIfc.Get(ctx, tenantID, metav1.GetOptions{})
		assert.NilError(t, err)

		assertThatExactlyTheseNamespacesExist(t, cf,
			clientNSName,
		)
		assertThatExactlyTheseTenantsExistInNamespace(t, cf, clientNSName /* none */)
	}
}

func assertThatExactlyTheseNamespacesExist(t *testing.T, cf *k8sfake.ClientFactory, expectedNamespaces ...string) {
	t.Helper()

	ctx := context.Background()
	nsList, err := cf.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	assert.NilError(t, err)
	dump := fmt.Sprintf("\n\n%v", spew.Sdump(nsList.Items))

	expected := make(map[string]bool, len(expectedNamespaces))
	for _, n := range expectedNamespaces {
		if n != "" {
			expected[n] = true
		}
	}
	actual := make(map[string]bool, len(nsList.Items))
	for _, item := range nsList.Items {
		actual[item.GetName()] = true
	}

	for n := range expected {
		assert.Assert(t, actual[n], "expected namespace %q is missing%s", n, dump)
	}
	for n := range actual {
		assert.Assert(t, expected[n], "found unexpected namespace %q%s", n, dump)
	}
}

func assertThatExactlyTheseTenantsExistInNamespace(t *testing.T, cf *k8sfake.ClientFactory, namespace string, expectedTenants ...string) {
	t.Helper()

	ctx := context.Background()
	tenantList, err := cf.StewardV1alpha1().Tenants(namespace).List(ctx, metav1.ListOptions{})
	assert.NilError(t, err)
	dump := fmt.Sprintf("\n\n%v", spew.Sdump(tenantList.Items))

	expected := make(map[string]bool, len(expectedTenants))
	for _, n := range expectedTenants {
		if n != "" {
			expected[n] = true
		}
	}
	actual := make(map[string]bool, len(tenantList.Items))
	for _, item := range tenantList.Items {
		if !item.GetDeletionTimestamp().IsZero() && len(item.GetFinalizers()) == 0 {
			// treat finalized object as deleted
			continue
		}
		actual[item.GetName()] = true
	}

	for n := range expected {
		assert.Assert(t, actual[n], "expected tenant %q in namespace %q is missing%s", n, namespace, dump)
	}
	for n := range actual {
		assert.Assert(t, expected[n], "found unexpected tenant %q in namespace %q%s", n, namespace, dump)
	}
}

func assertThatExactlyTheseFinalizersExist(t *testing.T, obj *metav1.ObjectMeta, expectedFinalizers ...string) {
	t.Helper()

	finalizers := obj.GetFinalizers()
	dump := fmt.Sprintf("\n\n%v", spew.Sdump(finalizers))

	expected := make(map[string]bool, len(finalizers))
	for _, n := range finalizers {
		expected[n] = true
	}
	actual := make(map[string]bool, len(finalizers))
	for _, n := range finalizers {
		actual[n] = true
	}

	for n := range expected {
		assert.Assert(t, actual[n], "expected finalizer %q is missing%s", n, dump)
	}
	for n := range actual {
		assert.Assert(t, expected[n], "found unexpected finalizer %q%s", n, dump)
	}
}

func startController(t *testing.T, cf *k8sfake.ClientFactory) (chan struct{}, *Controller) {
	stopCh := make(chan struct{}, 0)
	controller := NewController(cf, ControllerOpts{})
	controller.fetcher = k8s.NewClientBasedTenantFetcher(cf)
	cf.StewardInformerFactory().Start(stopCh)
	go start(t, controller, stopCh)
	cf.Sleep("Wait for controller")
	return stopCh, controller
}

func stopController(t *testing.T, stopCh chan struct{}) {
	t.Log("Trigger controller stop")
	stopCh <- struct{}{}
}

func runControllerForAWhile(t *testing.T, cf *k8sfake.ClientFactory) *Controller {
	stopCh, controller := startController(t, cf)
	defer stopController(t, stopCh)
	return controller
}

func start(t *testing.T, controller *Controller, stopCh chan struct{}) {
	if err := controller.Run(1, stopCh); err != nil {
		t.Logf("Error running controller %s", err.Error())
	}
}

func makeTenantKey(namespace string, tenantID string) string {
	return fmt.Sprintf("%s/%s", namespace, tenantID)
}

func sleep(duration string) {
	durationParsed, err := time.ParseDuration(duration)
	if err != nil {
		panic(err)
	}
	time.Sleep(durationParsed)
}

func waitForNextSync(t *testing.T, controller *Controller, previousSyncCount int64) {
	t.Helper()
	t.Log("waiting for tenant controller sync")
	for controller.getSyncCount() <= previousSyncCount {
		sleep("5ms")
	}
}
