package apply

import (
	"context"
	"fmt"

	"k8s.io/client-go/tools/cache"

	"kubevirt.io/kubevirt/pkg/controller"

	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"kubevirt.io/client-go/log"
	"kubevirt.io/kubevirt/pkg/virt-operator/resource/generate/rbac"
)

type RoleType int

const (
	TypeRole               RoleType = iota
	TypeClusterRole        RoleType = iota
	TypeRoleBinding        RoleType = iota
	TypeClusterRoleBinding RoleType = iota
)

func (r *Reconciler) createOrUpdateClusterRole(cr *rbacv1.ClusterRole, imageTag string, imageRegistry string, id string) error {
	return r.createOrUpdate(cr, imageTag, imageRegistry, id, TypeClusterRole, false)
}

func (r *Reconciler) createOrUpdateClusterRoleBinding(crb *rbacv1.ClusterRoleBinding, imageTag string, imageRegistry string, id string) error {
	return r.createOrUpdate(crb, imageTag, imageRegistry, id, TypeClusterRoleBinding, false)
}

func (r *Reconciler) createOrUpdateRole(role *rbacv1.Role, imageTag string, imageRegistry string, id string) error {
	return r.createOrUpdate(role, imageTag, imageRegistry, id, TypeRole, true)
}

func (r *Reconciler) createOrUpdateRoleBinding(rb *rbacv1.RoleBinding, imageTag string, imageRegistry string, id string) error {
	return r.createOrUpdate(rb, imageTag, imageRegistry, id, TypeRoleBinding, true)
}

func (r *Reconciler) createOrUpdate(role interface{},
	imageTag, imageRegistry, id string,
	roleType RoleType,
	avoidIfServiceAccount bool) (err error) {

	roleTypeName := getRoleTypeName(roleType)
	createRole := r.getRoleCreateFunction(role, roleType)
	updateRole := r.getRoleUpdateFunction(role, roleType)

	cachedRole, exists, _ := r.getRoleCache(roleType).Get(role)
	roleMeta := getRoleMetaObject(role, roleType)
	if avoidIfServiceAccount && !r.stores.ServiceMonitorEnabled && (roleMeta.Name == rbac.MONITOR_SERVICEACCOUNT_NAME) {
		return nil
	}

	injectOperatorMetadata(r.kv, roleMeta, imageTag, imageRegistry, id, true)
	if !exists {
		// Create non existent
		err = createRole()
		if err != nil {
			return fmt.Errorf("unable to create %v %+v: %v", roleTypeName, role, err)
		}
		log.Log.V(2).Infof("%v %v created", roleTypeName, roleMeta.GetName())
	} else if !objectMatchesVersion(getRoleMetaObject(cachedRole, roleType), imageTag, imageRegistry, id, r.kv.GetGeneration()) {
		// Update existing, we don't need to patch for rbac rules.
		err = updateRole()
		if err != nil {
			return fmt.Errorf("unable to update %v %+v: %v", roleTypeName, role, err)
		}
		log.Log.V(2).Infof("%v %v updated", roleTypeName, roleMeta.GetName())

	} else {
		log.Log.V(4).Infof("%v %v already exists", roleTypeName, roleMeta.GetName())
	}

	return nil
}

func (r *Reconciler) getRoleCreateFunction(obj interface{}, roleType RoleType) (createFunc func() error) {

	rbacObj := r.clientset.RbacV1()
	namespace := r.kv.Namespace

	raiseExpectation := func(exp *controller.UIDTrackingControllerExpectations) {
		exp.RaiseExpectations(r.kvKey, 1, 0)
	}
	lowerExpectationIfErr := func(exp *controller.UIDTrackingControllerExpectations, err error) {
		if err != nil {
			exp.LowerExpectations(r.kvKey, 1, 0)
		}
	}

	switch roleType {
	case TypeRole:
		role := obj.(*rbacv1.Role)

		createFunc = func() error {
			raiseExpectation(r.expectations.Role)
			_, err := rbacObj.Roles(namespace).Create(context.Background(), role, metav1.CreateOptions{})
			lowerExpectationIfErr(r.expectations.Role, err)
			return err
		}
	case TypeClusterRole:
		role := obj.(*rbacv1.ClusterRole)

		createFunc = func() error {
			raiseExpectation(r.expectations.ClusterRole)
			_, err := rbacObj.ClusterRoles().Create(context.Background(), role, metav1.CreateOptions{})
			lowerExpectationIfErr(r.expectations.ClusterRole, err)
			return err
		}
	case TypeRoleBinding:
		roleBinding := obj.(*rbacv1.RoleBinding)

		createFunc = func() error {
			raiseExpectation(r.expectations.RoleBinding)
			_, err := rbacObj.RoleBindings(namespace).Create(context.Background(), roleBinding, metav1.CreateOptions{})
			lowerExpectationIfErr(r.expectations.RoleBinding, err)
			return err
		}
	case TypeClusterRoleBinding:
		roleBinding := obj.(*rbacv1.ClusterRoleBinding)

		createFunc = func() error {
			raiseExpectation(r.expectations.ClusterRoleBinding)
			_, err := rbacObj.ClusterRoleBindings().Create(context.Background(), roleBinding, metav1.CreateOptions{})
			lowerExpectationIfErr(r.expectations.ClusterRoleBinding, err)
			return err
		}
	}

	return
}

func (r *Reconciler) getRoleUpdateFunction(obj interface{}, roleType RoleType) (updateFunc func() (err error)) {
	rbacObj := r.clientset.RbacV1()
	namespace := r.kv.Namespace

	switch roleType {
	case TypeRole:
		role := obj.(*rbacv1.Role)

		updateFunc = func() (err error) {
			_, err = rbacObj.Roles(namespace).Update(context.Background(), role, metav1.UpdateOptions{})
			return err
		}
	case TypeClusterRole:
		role := obj.(*rbacv1.ClusterRole)

		updateFunc = func() (err error) {
			_, err = rbacObj.ClusterRoles().Update(context.Background(), role, metav1.UpdateOptions{})
			return err
		}
	case TypeRoleBinding:
		roleBinding := obj.(*rbacv1.RoleBinding)

		updateFunc = func() (err error) {
			_, err = rbacObj.RoleBindings(namespace).Update(context.Background(), roleBinding, metav1.UpdateOptions{})
			return err
		}
	case TypeClusterRoleBinding:
		roleBinding := obj.(*rbacv1.ClusterRoleBinding)

		updateFunc = func() (err error) {
			_, err = rbacObj.ClusterRoleBindings().Update(context.Background(), roleBinding, metav1.UpdateOptions{})
			return err
		}
	}

	return
}

func getRoleTypeName(roleType RoleType) (name string) {
	switch roleType {
	case TypeRole:
		name = "role"
	case TypeClusterRole:
		name = "clusterrole"
	case TypeRoleBinding:
		name = "rolebinding"
	case TypeClusterRoleBinding:
		name = "clusterrolebinding"
	}

	return
}

func getRoleMetaObject(role interface{}, roleType RoleType) (meta *metav1.ObjectMeta) {
	switch roleType {
	case TypeRole:
		role := role.(*rbacv1.Role)
		meta = &role.ObjectMeta
	case TypeClusterRole:
		role := role.(*rbacv1.ClusterRole)
		meta = &role.ObjectMeta
	case TypeRoleBinding:
		roleBinding := role.(*rbacv1.RoleBinding)
		meta = &roleBinding.ObjectMeta
	case TypeClusterRoleBinding:
		roleBinding := role.(*rbacv1.ClusterRoleBinding)
		meta = &roleBinding.ObjectMeta
	}

	return
}

func (r *Reconciler) getRoleCache(roleType RoleType) (cache cache.Store) {
	switch roleType {
	case TypeRole:
		cache = r.stores.RoleCache
	case TypeClusterRole:
		cache = r.stores.ClusterRoleCache
	case TypeRoleBinding:
		cache = r.stores.RoleBindingCache
	case TypeClusterRoleBinding:
		cache = r.stores.ClusterRoleBindingCache
	}

	return cache
}
