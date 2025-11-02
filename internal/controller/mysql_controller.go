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
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=persistentvolumeclaims,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete

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

	// Create or update PVC
	pvc := r.pvcForMySQL(mysql, sanitizedName)
	if err := controllerutil.SetControllerReference(mysql, pvc, r.Scheme); err != nil {
		return ctrl.Result{}, err
	}
	foundPVC := &corev1.PersistentVolumeClaim{}
	err = r.Get(ctx, types.NamespacedName{Name: pvc.Name, Namespace: pvc.Namespace}, foundPVC)
	if err != nil && errors.IsNotFound(err) {
		log.Info("Creating a new PVC", "PVC.Namespace", pvc.Namespace, "PVC.Name", pvc.Name)
		err = r.Create(ctx, pvc)
		if err != nil {
			log.Error(err, "Failed to create new PVC")
			return ctrl.Result{}, err
		}
	}

	// Create or update Deployment
	deployment := r.deploymentForMySQL(mysql, sanitizedName)
	if err := controllerutil.SetControllerReference(mysql, deployment, r.Scheme); err != nil {
		return ctrl.Result{}, err
	}
	foundDeployment := &appsv1.Deployment{}
	err = r.Get(ctx, types.NamespacedName{Name: deployment.Name, Namespace: deployment.Namespace}, foundDeployment)
	if err != nil && errors.IsNotFound(err) {
		log.Info("Creating a new Deployment", "Deployment.Namespace", deployment.Namespace, "Deployment.Name", deployment.Name)
		err = r.Create(ctx, deployment)
		if err != nil {
			log.Error(err, "Failed to create new Deployment")
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

	// Update status
	mysql.Status.Phase = "Running"
	mysql.Status.Ready = true
	mysql.Status.Message = "MySQL instance is running"
	if err := r.Status().Update(ctx, mysql); err != nil {
		log.Error(err, "Failed to update MySQL status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// sanitizeName ensures the name is DNS-1035 compliant by replacing dots with dashes
func sanitizeName(name string) string {
	return strings.ReplaceAll(name, ".", "-")
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

func (r *MySQLReconciler) pvcForMySQL(m *databasev1alpha1.MySQL, sanitizedName string) *corev1.PersistentVolumeClaim {
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      sanitizedName + "-pvc",
			Namespace: m.Namespace,
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{
				corev1.ReadWriteOnce,
			},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse(m.Spec.StorageSize),
				},
			},
		},
	}
	return pvc
}

func (r *MySQLReconciler) deploymentForMySQL(m *databasev1alpha1.MySQL, sanitizedName string) *appsv1.Deployment {
	replicas := int32(1)

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      sanitizedName,
			Namespace: m.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": sanitizedName,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": sanitizedName,
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "mysql",
							Image: fmt.Sprintf("mysql:%s", m.Spec.Version),
							Ports: []corev1.ContainerPort{
								{
									ContainerPort: 3306,
									Name:          "mysql",
								},
							},
							Env: []corev1.EnvVar{
								{
									Name: "MYSQL_ROOT_PASSWORD",
									ValueFrom: &corev1.EnvVarSource{
										SecretKeyRef: &corev1.SecretKeySelector{
											LocalObjectReference: corev1.LocalObjectReference{
												Name: sanitizedName + "-secret",
											},
											Key: "root-password",
										},
									},
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "mysql-data",
									MountPath: "/var/lib/mysql",
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "mysql-data",
							VolumeSource: corev1.VolumeSource{
								PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: sanitizedName + "-pvc",
								},
							},
						},
					},
				},
			},
		},
	}
	return deployment
}

func (r *MySQLReconciler) serviceForMySQL(m *databasev1alpha1.MySQL, sanitizedName string) *corev1.Service {
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      sanitizedName + "-service",
			Namespace: m.Namespace,
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				"app": sanitizedName,
			},
			Ports: []corev1.ServicePort{
				{
					Port:     3306,
					Name:     "mysql",
					Protocol: corev1.ProtocolTCP,
				},
			},
			Type: corev1.ServiceTypeClusterIP,
		},
	}
	return service
}

// SetupWithManager sets up the controller with the Manager.
func (r *MySQLReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&databasev1alpha1.MySQL{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.PersistentVolumeClaim{}).
		Owns(&corev1.Secret{}).
		Complete(r)
}
