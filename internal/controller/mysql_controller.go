package controller

import (
	"context"
	"fmt"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	databasev1alpha1 "github.com/sudhir-u/my-mysql-operator/api/v1alpha1"
)

// MySQLReconciler reconciles a MySQL object
type MySQLReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=database.mycompany.com,resources=mysqls,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=database.mycompany.com,resources=mysqls/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=database.mycompany.com,resources=mysqls/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch;patch;update
// +kubebuilder:rbac:groups=core,resources=nodes,verbs=get;list;watch

func (r *MySQLReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// Fetch the MySQL instance
	mysql := &databasev1alpha1.MySQL{}
	err := r.Get(ctx, req.NamespacedName, mysql)
	if err != nil {
		if errors.IsNotFound(err) {
			log.Info("MySQL resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to get MySQL")
		return ctrl.Result{}, err
	}

	// Use sanitized name for all resources
	sanitizedName := sanitizeName(mysql.Name)

	// Create or update Secret
	secret := r.secretForMySQL(mysql, sanitizedName)
	if err := controllerutil.SetControllerReference(mysql, secret, r.Scheme); err != nil {
		return ctrl.Result{}, err
	}
	foundSecret := &corev1.Secret{}
	err = r.Get(ctx, types.NamespacedName{Name: secret.Name, Namespace: secret.Namespace}, foundSecret)
	if err != nil && errors.IsNotFound(err) {
		log.Info("Creating a new Secret", "Secret.Namespace", secret.Namespace, "Secret.Name", secret.Name)
		err = r.Create(ctx, secret)
		if err != nil {
			log.Error(err, "Failed to create new Secret")
			return ctrl.Result{}, err
		}
	}

	// Create or update headless Service (required for StatefulSet)
	headlessSvc := r.headlessServiceForMySQL(mysql, sanitizedName)
	if err := controllerutil.SetControllerReference(mysql, headlessSvc, r.Scheme); err != nil {
		return ctrl.Result{}, err
	}
	foundHeadless := &corev1.Service{}
	err = r.Get(ctx, types.NamespacedName{Name: headlessSvc.Name, Namespace: headlessSvc.Namespace}, foundHeadless)
	if err != nil && errors.IsNotFound(err) {
		log.Info("Creating headless Service", "Service.Namespace", headlessSvc.Namespace, "Service.Name", headlessSvc.Name)
		if err = r.Create(ctx, headlessSvc); err != nil {
			log.Error(err, "Failed to create headless Service")
			return ctrl.Result{}, err
		}
	}

	// Create or update StatefulSet
	sts := r.statefulSetForMySQL(mysql, sanitizedName)
	if err := controllerutil.SetControllerReference(mysql, sts, r.Scheme); err != nil {
		return ctrl.Result{}, err
	}
	foundSTS := &appsv1.StatefulSet{}
	err = r.Get(ctx, types.NamespacedName{Name: sts.Name, Namespace: sts.Namespace}, foundSTS)
	if err != nil && errors.IsNotFound(err) {
		log.Info("Creating StatefulSet", "StatefulSet.Namespace", sts.Namespace, "StatefulSet.Name", sts.Name)
		if err = r.Create(ctx, sts); err != nil {
			log.Error(err, "Failed to create StatefulSet")
			return ctrl.Result{}, err
		}
	}

	// Create or update Service
	service := r.serviceForMySQL(mysql, sanitizedName)
	if err := controllerutil.SetControllerReference(mysql, service, r.Scheme); err != nil {
		return ctrl.Result{}, err
	}
	foundService := &corev1.Service{}
	err = r.Get(ctx, types.NamespacedName{Name: service.Name, Namespace: service.Namespace}, foundService)
	if err != nil && errors.IsNotFound(err) {
		log.Info("Creating a new Service", "Service.Namespace", service.Namespace, "Service.Name", service.Name)
		err = r.Create(ctx, service)
		if err != nil {
			log.Error(err, "Failed to create new Service")
			return ctrl.Result{}, err
		}
	}

	// Failover: ensure primary designation and pod labels (mysql-role=primary|standby); run when replicas >= 2
	if err := r.ensurePrimaryAndFailover(ctx, mysql, sanitizedName); err != nil {
		log.Error(err, "Failed to ensure primary/failover")
		return ctrl.Result{}, err
	}

	// Update status based on actual StatefulSet and Pod state
	if err := r.updateMySQLStatus(ctx, mysql, sanitizedName); err != nil {
		log.Error(err, "Failed to update MySQL status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// sanitizeName ensures the name is DNS-1035 compliant by replacing dots with dashes
func sanitizeName(name string) string {
	return strings.ReplaceAll(name, ".", "-")
}

const (
	labelMySQLRole = "mysql-role"
	rolePrimary    = "primary"
	roleStandby    = "standby"
)

// ensurePrimaryAndFailover: for replicas>=2, designates primary, monitors health, fences on failure, promotes standby, and updates pod labels so the primary Service routes to the current primary.
func (r *MySQLReconciler) ensurePrimaryAndFailover(ctx context.Context, mysql *databasev1alpha1.MySQL, sanitizedName string) error {
	log := log.FromContext(ctx)
	replicas := int32(1)
	if mysql.Spec.Replicas != nil && *mysql.Spec.Replicas >= 1 && *mysql.Spec.Replicas <= 2 {
		replicas = *mysql.Spec.Replicas
	}

	podList := &corev1.PodList{}
	if err := r.List(ctx, podList, client.InNamespace(mysql.Namespace), client.MatchingLabels(map[string]string{"app": sanitizedName})); err != nil {
		return err
	}
	if len(podList.Items) == 0 {
		return nil
	}

	// Single replica: ensure the only pod has mysql-role=primary; clear status.primaryPodName or set to it
	if replicas == 1 {
		pod := &podList.Items[0]
		if pod.Labels[labelMySQLRole] != rolePrimary {
			if err := r.patchPodLabel(ctx, pod, labelMySQLRole, rolePrimary); err != nil {
				return err
			}
		}
		return r.statusUpdatePrimaryPod(ctx, mysql.Namespace, mysql.Name, pod.Name)
	}

	// Two replicas: primary/standby failover logic
	currentPrimary := mysql.Status.PrimaryPodName
	readyPods := make([]corev1.Pod, 0, 2)
	for i := range podList.Items {
		if r.isPodReady(&podList.Items[i]) {
			readyPods = append(readyPods, podList.Items[i])
		}
	}

	// Designate initial primary (lowest ordinal that is ready)
	if currentPrimary == "" {
		if len(readyPods) == 0 {
			return nil
		}
		chosen := r.podWithLowestOrdinal(readyPods)
		if err := r.setPrimaryLabels(ctx, mysql.Namespace, sanitizedName, chosen.Name, podList.Items); err != nil {
			return err
		}
		return r.statusUpdatePrimaryPod(ctx, mysql.Namespace, mysql.Name, chosen.Name)
	}

	// Find current primary pod
	var primaryPod *corev1.Pod
	for i := range podList.Items {
		if podList.Items[i].Name == currentPrimary {
			primaryPod = &podList.Items[i]
			break
		}
	}

	// Primary pod missing or not ready or node not ready -> failover
	needFailover := primaryPod == nil || !r.isPodReady(primaryPod)
	if primaryPod != nil && primaryPod.Spec.NodeName != "" {
		if nodeReady, _ := r.isNodeReady(ctx, primaryPod.Spec.NodeName); !nodeReady {
			needFailover = true
		}
	}

	if needFailover {
		// Promote standby: pick another ready pod (exclude current primary)
		var newPrimary *corev1.Pod
		for i := range readyPods {
			if readyPods[i].Name != currentPrimary {
				newPrimary = &readyPods[i]
				break
			}
		}
		if newPrimary == nil {
			log.Info("Failover needed but no other ready pod", "currentPrimary", currentPrimary)
			return nil
		}
		log.Info("Failover: promoting standby to primary", "oldPrimary", currentPrimary, "newPrimary", newPrimary.Name)
		// Fencing: relabel old primary to standby (so Service no longer routes to it)
		// Promotion: set new primary pod label
		if err := r.setPrimaryLabels(ctx, mysql.Namespace, sanitizedName, newPrimary.Name, podList.Items); err != nil {
			return err
		}
		return r.statusUpdatePrimaryPod(ctx, mysql.Namespace, mysql.Name, newPrimary.Name)
	}

	// Ensure labels are correct
	return r.setPrimaryLabels(ctx, mysql.Namespace, sanitizedName, currentPrimary, podList.Items)
}

func (r *MySQLReconciler) isPodReady(pod *corev1.Pod) bool {
	for _, c := range pod.Status.Conditions {
		if c.Type == corev1.PodReady && c.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

func (r *MySQLReconciler) isNodeReady(ctx context.Context, nodeName string) (bool, error) {
	node := &corev1.Node{}
	if err := r.Get(ctx, types.NamespacedName{Name: nodeName}, node); err != nil {
		return false, err
	}
	for _, c := range node.Status.Conditions {
		if c.Type == corev1.NodeReady && c.Status == corev1.ConditionTrue {
			return true, nil
		}
	}
	return false, nil
}

func (r *MySQLReconciler) podWithLowestOrdinal(pods []corev1.Pod) *corev1.Pod {
	if len(pods) == 0 {
		return nil
	}
	out := &pods[0]
	for i := 1; i < len(pods); i++ {
		if pods[i].Name < out.Name {
			out = &pods[i]
		}
	}
	return out
}

func (r *MySQLReconciler) setPrimaryLabels(ctx context.Context, ns, sanitizedName, primaryName string, pods []corev1.Pod) error {
	for i := range pods {
		role := roleStandby
		if pods[i].Name == primaryName {
			role = rolePrimary
		}
		if pods[i].Labels[labelMySQLRole] != role {
			if err := r.patchPodLabel(ctx, &pods[i], labelMySQLRole, role); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *MySQLReconciler) patchPodLabel(ctx context.Context, pod *corev1.Pod, key, value string) error {
	payload := []byte(fmt.Sprintf(`{"metadata":{"labels":{%q:%q}}}`, key, value))
	return r.Patch(ctx, pod, client.RawPatch(types.StrategicMergePatchType, payload))
}

func (r *MySQLReconciler) statusUpdatePrimaryPod(ctx context.Context, ns, name, primaryPodName string) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		mysql := &databasev1alpha1.MySQL{}
		if err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: ns}, mysql); err != nil {
			return err
		}
		mysql.Status.PrimaryPodName = primaryPodName
		return r.Status().Update(ctx, mysql)
	})
}

// updateMySQLStatus updates the MySQL status based on the actual state of the StatefulSet and Pods.
// Uses retry-on-conflict to handle concurrent updates and resource version conflicts.
func (r *MySQLReconciler) updateMySQLStatus(ctx context.Context, mysql *databasev1alpha1.MySQL, sanitizedName string) error {
	log := log.FromContext(ctx)

	// Check if StatefulSet exists
	sts := &appsv1.StatefulSet{}
	err := r.Get(ctx, types.NamespacedName{Name: sanitizedName, Namespace: mysql.Namespace}, sts)
	if err != nil {
		if errors.IsNotFound(err) {
			return r.statusUpdateWithRetry(ctx, mysql.Namespace, mysql.Name, "Pending", false, "StatefulSet not found, resources are being created")
		}
		return err
	}

	// Check StatefulSet status
	availableReplicas := sts.Status.AvailableReplicas
	readyReplicas := sts.Status.ReadyReplicas
	replicas := sts.Status.Replicas

	// If no replicas are available yet, status is Pending
	if availableReplicas == 0 {
		msg := "Waiting for StatefulSet to create pods"
		if replicas > 0 {
			msg = "MySQL pod is starting up"
		}
		return r.statusUpdateWithRetry(ctx, mysql.Namespace, mysql.Name, "Pending", false, msg)
	}

	// Check Pod status to get more detailed information
	podList := &corev1.PodList{}
	err = r.List(ctx, podList, client.InNamespace(mysql.Namespace), client.MatchingLabels(map[string]string{"app": sanitizedName}))
	if err != nil {
		log.Error(err, "Failed to list pods for status check")
	} else if len(podList.Items) > 0 {
		pod := podList.Items[0] // First pod (e.g. mysql-0 for single replica)

		switch pod.Status.Phase {
		case corev1.PodPending:
			return r.statusUpdateWithRetry(ctx, mysql.Namespace, mysql.Name, "Pending", false, "Pod is pending, waiting for resources")
		case corev1.PodFailed:
			msg := "Pod has failed"
			for _, containerStatus := range pod.Status.ContainerStatuses {
				if containerStatus.State.Waiting != nil {
					msg = fmt.Sprintf("Pod failed: %s", containerStatus.State.Waiting.Message)
					break
				}
				if containerStatus.State.Terminated != nil {
					msg = fmt.Sprintf("Pod terminated: %s", containerStatus.State.Terminated.Message)
					break
				}
			}
			return r.statusUpdateWithRetry(ctx, mysql.Namespace, mysql.Name, "Failed", false, msg)
		case corev1.PodRunning:
			isReady := false
			for _, condition := range pod.Status.Conditions {
				if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
					isReady = true
					break
				}
			}
			if isReady && readyReplicas > 0 {
				return r.statusUpdateWithRetry(ctx, mysql.Namespace, mysql.Name, "Running", true, "MySQL instance is running and ready")
			}
			return r.statusUpdateWithRetry(ctx, mysql.Namespace, mysql.Name, "Pending", false, "Pod is running but not ready yet")
		}
	}

	// Fallback: Use StatefulSet status
	if readyReplicas > 0 && availableReplicas > 0 {
		return r.statusUpdateWithRetry(ctx, mysql.Namespace, mysql.Name, "Running", true, "MySQL instance is running")
	}
	return r.statusUpdateWithRetry(ctx, mysql.Namespace, mysql.Name, "Pending", false, "Waiting for MySQL to become ready")
}

// statusUpdateWithRetry performs a status update with retry-on-conflict to handle
// concurrent modifications (e.g. rapid re-reconciliation or external updates).
func (r *MySQLReconciler) statusUpdateWithRetry(ctx context.Context, ns, name, phase string, ready bool, message string) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		mysql := &databasev1alpha1.MySQL{}
		if err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: ns}, mysql); err != nil {
			return err
		}
		mysql.Status.Phase = phase
		mysql.Status.Ready = ready
		mysql.Status.Message = message
		return r.Status().Update(ctx, mysql)
	})
}

func (r *MySQLReconciler) secretForMySQL(m *databasev1alpha1.MySQL, sanitizedName string) *corev1.Secret {
	password := m.Spec.RootPassword
	if password == "" {
		password = "changeme" // Default password for demo
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      sanitizedName + "-secret",
			Namespace: m.Namespace,
		},
		StringData: map[string]string{
			"root-password": password,
		},
	}
	return secret
}

// headlessServiceForMySQL returns a headless Service required by the StatefulSet (stable network identity).
func (r *MySQLReconciler) headlessServiceForMySQL(m *databasev1alpha1.MySQL, sanitizedName string) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      sanitizedName + "-headless",
			Namespace: m.Namespace,
		},
		Spec: corev1.ServiceSpec{
			ClusterIP: corev1.ClusterIPNone,
			Selector:  map[string]string{"app": sanitizedName},
			Ports: []corev1.ServicePort{
				{Port: 3306, Name: "mysql", Protocol: corev1.ProtocolTCP},
			},
		},
	}
}

// statefulSetForMySQL returns a StatefulSet for MySQL with per-pod PVC via volumeClaimTemplates.
func (r *MySQLReconciler) statefulSetForMySQL(m *databasev1alpha1.MySQL, sanitizedName string) *appsv1.StatefulSet {
	replicas := int32(1)
	if m.Spec.Replicas != nil && *m.Spec.Replicas >= 1 && *m.Spec.Replicas <= 2 {
		replicas = *m.Spec.Replicas
	}
	return &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      sanitizedName,
			Namespace: m.Namespace,
		},
		Spec: appsv1.StatefulSetSpec{
			ServiceName: sanitizedName + "-headless",
			Replicas:    &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": sanitizedName},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": sanitizedName},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "mysql",
							Image: fmt.Sprintf("mysql:%s", m.Spec.Version),
							Ports: []corev1.ContainerPort{
								{ContainerPort: 3306, Name: "mysql"},
							},
							Env: []corev1.EnvVar{
								{
									Name: "MYSQL_ROOT_PASSWORD",
									ValueFrom: &corev1.EnvVarSource{
										SecretKeyRef: &corev1.SecretKeySelector{
											LocalObjectReference: corev1.LocalObjectReference{Name: sanitizedName + "-secret"},
											Key:                  "root-password",
										},
									},
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{Name: "mysql-data", MountPath: "/var/lib/mysql"},
							},
						},
					},
					// Volumes for volumeClaimTemplates are injected by the StatefulSet controller
				},
			},
			VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "mysql-data"},
					Spec: corev1.PersistentVolumeClaimSpec{
						AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
						Resources: corev1.VolumeResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceStorage: resource.MustParse(m.Spec.StorageSize),
							},
						},
					},
				},
			},
		},
	}
}

// serviceForMySQL returns the primary Service; selector uses mysql-role=primary so
// traffic goes only to the current primary pod (updated by failover logic).
func (r *MySQLReconciler) serviceForMySQL(m *databasev1alpha1.MySQL, sanitizedName string) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      sanitizedName + "-service",
			Namespace: m.Namespace,
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				"app":         sanitizedName,
				"mysql-role": "primary",
			},
			Ports: []corev1.ServicePort{
				{Port: 3306, Name: "mysql", Protocol: corev1.ProtocolTCP},
			},
			Type: corev1.ServiceTypeClusterIP,
		},
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *MySQLReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&databasev1alpha1.MySQL{}).
		Owns(&appsv1.StatefulSet{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.Secret{}).
		Complete(r)
}
