/*
Copyright 2024.

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

package controllers

import (
	"context"
	pgbackrestv1alpha1 "github.com/SamuelBECK1/pgbackrest-operator/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
)

// RestoreReconciler reconciles a Restore object
type RestoreReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=pgbackrest.pgbackrest,resources=restores,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=pgbackrest.pgbackrest,resources=restores/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=pgbackrest.pgbackrest,resources=restores/finalizers,verbs=update
//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;
//+kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Restore object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.14.1/pkg/reconcile
func (r *RestoreReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrllog.FromContext(ctx)
	log.Info("starting reconciliation")
	// TODO(user): your logic here
	rest := &pgbackrestv1alpha1.Restore{}
	err := r.Get(ctx, req.NamespacedName, rest)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			log.Info("Restore resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		log.Error(err, "Failed to get Restore")
		return ctrl.Result{}, err
	}
	foundDeploy := &appsv1.Deployment{}
	err = r.Get(ctx, types.NamespacedName{Name: rest.Name, Namespace: rest.Namespace}, foundDeploy)
	if err != nil && errors.IsNotFound(err) {
		// Define a new deployment
		dep := r.deploymentForRestore(rest)
		log.Info("Creating a new Deployment", "Deployment.Namespace", dep.Namespace, "Deployment.Name", dep.Name)
		err = r.Create(ctx, dep)
		if err != nil {
			log.Error(err, "Failed to create new Deployment", "Deployment.Namespace", dep.Namespace, "Deployment.Name", dep.Name)
			return ctrl.Result{}, err
		}
		// Deployment created successfully - return and requeue
		return ctrl.Result{Requeue: true}, nil
	} else if err != nil {
		log.Error(err, "Failed to get Deployment")
		return ctrl.Result{}, err
	}
	foundCm := &corev1.ConfigMap{}
	err = r.Get(ctx, types.NamespacedName{Name: rest.Name + "pgbackrest-pgadmin-config", Namespace: rest.Namespace}, foundCm)
	if err != nil && errors.IsNotFound(err) {
		// Define a new deployment
		cm := r.configMapForRestore(rest)
		log.Info("Creating a new cm", "cm.Namespace", cm.Namespace, "cm.Name", cm.Name)
		err = r.Create(ctx, cm)
		if err != nil {
			log.Error(err, "Failed to create new cm", "cm.Namespace", cm.Namespace, "cm.Name", cm.Name)
			return ctrl.Result{}, err
		}
		// Deployment created successfully - return and requeue
		return ctrl.Result{Requeue: true}, nil
	} else if err != nil {
		log.Error(err, "Failed to get cm")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *RestoreReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&pgbackrestv1alpha1.Restore{}).
		Complete(r)
}

func (r *RestoreReconciler) configMapForRestore(rest *pgbackrestv1alpha1.Restore) *corev1.ConfigMap {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      rest.Name + "pgbackrest-pgadmin-config",
			Namespace: rest.Namespace,
		},
		Data: map[string]string{
			"pgpass": "127.0.0.1:54332:" + rest.Spec.DBName + ":" + rest.Spec.DBUser + ":" + rest.Spec.DBPassword,
		},
	}
	ctrl.SetControllerReference(rest, cm, r.Scheme)
	return cm
}

func (r *RestoreReconciler) deploymentForRestore(rest *pgbackrestv1alpha1.Restore) *appsv1.Deployment {
	ls := labelsForRestore(rest.Name)
	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      rest.Name,
			Namespace: rest.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: ls,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: ls,
				},
				Spec: corev1.PodSpec{
					InitContainers: []corev1.Container{{
						Image:   "busybox",
						Name:    "busybox-setperms",
						Command: []string{"sh", "-c", "cp /pgpass /tmp/files && chown 5050:5050 /tmp/files/pgpass && chmod 600 /tmp/files/pgpass"},
						VolumeMounts: []corev1.VolumeMount{{
							Name:      "pgbackrest-pgadmin-config-pgpass",
							MountPath: "/pgpass",
							SubPath:   "pgpass",
						}, {
							Name:      "pgpass-emptydir",
							MountPath: "/tmp/files/",
						}},
					}},
					Containers: []corev1.Container{{
						Image: rest.Spec.PgBackRestImage,
						Name:  "pgbackrest",
						ReadinessProbe: &corev1.Probe{
							ProbeHandler: corev1.ProbeHandler{
								Exec: &corev1.ExecAction{
									Command: []string{"sh", "-c", "exec pg_isready -U 'postgres' -h 127.0.0.1 -p 5432"},
								},
							},
							InitialDelaySeconds: 30,
							FailureThreshold:    6,
							SuccessThreshold:    1,
							PeriodSeconds:       10,
							TimeoutSeconds:      5,
						},
						ImagePullPolicy: corev1.PullIfNotPresent,
						Env: []corev1.EnvVar{{
							Name:  "PGBACKREST_RESTORE_TARGET_TIME",
							Value: rest.Spec.RestoreTimestamp,
						}, {
							Name:  "PGBACKREST_S3_ENDPOINT",
							Value: rest.Spec.S3Endpoint,
						}, {
							Name:  "PGBACKREST_S3_KEY",
							Value: rest.Spec.S3Key,
						}, {
							Name:  "PGBACKREST_S3_KEY_SECRET",
							Value: rest.Spec.S3KeySecret,
						}, {
							Name:  "PGBACKREST_S3_BUCKET",
							Value: rest.Spec.S3Bucket,
						}},
					}},
					Volumes: []corev1.Volume{{
						Name: "pgpass-emptydir",
						VolumeSource: corev1.VolumeSource{
							EmptyDir: &corev1.EmptyDirVolumeSource{},
						},
					}, {
						Name: "pgbackrest-pgadmin-config-pgpass",
						VolumeSource: corev1.VolumeSource{
							ConfigMap: &corev1.ConfigMapVolumeSource{
								LocalObjectReference: corev1.LocalObjectReference{
									Name: rest.Name + "pgbackrest-pgadmin-config",
								},
								Items: []corev1.KeyToPath{{
									Key:  "pgpass",
									Path: "pgpass",
								}},
							},
						},
					}},
				},
			},
		},
	}

	ctrl.SetControllerReference(rest, dep, r.Scheme)
	return dep
}

func labelsForRestore(name string) map[string]string {
	return map[string]string{"app": "restore", "restore_cr": name}
}

func getPodNames(pods []corev1.Pod) []string {
	var podNames []string
	for _, pod := range pods {
		podNames = append(podNames, pod.Name)
	}
	return podNames
}
