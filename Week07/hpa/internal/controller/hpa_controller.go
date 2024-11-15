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

package controller

import (
	"context"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	autoscalv1 "hpa/api/v1"

	cron "github.com/robfig/cron/v3"

	appsv1 "k8s.io/api/apps/v1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// HpaReconciler reconciles a Hpa object
type HpaReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=autoscal.aiops.org,resources=hpas,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=autoscal.aiops.org,resources=hpas/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=autoscal.aiops.org,resources=hpas/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Hpa object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.19.0/pkg/reconcile
func (r *HpaReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	log.Info("Reconciling CronHPA")
	var cronhpa autoscalv1.Hpa

	// 从命名空间中拿到 HPA 资源对象
	if err := r.Get(ctx, req.NamespacedName, &cronhpa); err != nil {
		if errors.IsNotFound(err) {
			log.Info("CronHPA resource not found. Ignoring since object must be deleted")
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}

	now := time.Now()
	var earliestNextRunTime *time.Time

	// 遍历 jobs，检查调度时间并更新目标工作负载副本数
	for _, job := range cronhpa.Spec.Jobs {
		lastRunTime := cronhpa.Status.LastRuntimes[job.Name]
		// 计算上次运行时间之后的下一个调度时间
		nextScheduledTime, err := r.getNextScheduledTime(job.Schedule, lastRunTime.Time)
		if err != nil {
			log.Error(err, "Failed to calculate next scheduled time")
			return reconcile.Result{}, err
		}

		log.Info("Job info", "name", job.Name, "lastRunTime", lastRunTime, "nextScheduledTime", nextScheduledTime, "now", now)

		// 检查当前时间是否已经到达或超过了计划的运行时间
		if now.After(nextScheduledTime) || now.Equal(nextScheduledTime) {
			// 更新副本数
			log.Info("Updating deployment replicas", "name", cronhpa.Spec.ScaleTarget.Name, "targetSize", job.Size)
			if err := r.updateDeploymentReplicas(ctx, &cronhpa, cronhpa.Spec.ScaleTarget, job); err != nil {
				return reconcile.Result{}, err
			}

			// 更新状态
			cronhpa.Status.CurrentReplicas = job.Size
			cronhpa.Status.LastScaleTime = &metav1.Time{Time: now}

			// 更新作业的最后运行时间
			if cronhpa.Status.LastRuntimes == nil {
				cronhpa.Status.LastRuntimes = make(map[string]metav1.Time)
			}
			cronhpa.Status.LastRuntimes[job.Name] = metav1.Time{Time: now}

			// 计算下一次运行时间（从现在开始）
			nextRunTime, _ := r.getNextScheduledTime(job.Schedule, now)
			if earliestNextRunTime == nil || nextRunTime.Before(*earliestNextRunTime) {
				earliestNextRunTime = &nextRunTime
			}
		} else {
			// 如果当前时间未到达计划时间，将这个时间作为下一次运行时间
			if earliestNextRunTime == nil || nextScheduledTime.Before(*earliestNextRunTime) {
				earliestNextRunTime = &nextScheduledTime
			}
		}
	}

	// 更新 CronHPA 实例状态
	if err := r.Status().Update(ctx, &cronhpa); err != nil {
		return reconcile.Result{}, err
	}

	// 如果有下一次运行时间，设置重新入队
	if earliestNextRunTime != nil {
		requeueAfter := earliestNextRunTime.Sub(now)
		if requeueAfter < 0 {
			requeueAfter = time.Second // 如果计算出的时间已经过去，则在1秒后重新入队
		}
		log.Info("Requeue after", "time", requeueAfter)
		return reconcile.Result{RequeueAfter: requeueAfter}, nil
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *HpaReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&autoscalv1.Hpa{}).
		Named("hpa").
		Complete(r)
}

// updateDeploymentReplicas 更新目标工作负载的副本数
func (r *HpaReconciler) updateDeploymentReplicas(ctx context.Context, cronhpa *autoscalv1.Hpa, scaleTargetRef autoscalv1.ScaleTarget, job autoscalv1.JobSpec) error {
	log := log.FromContext(ctx)

	// 创建 deployment 对象
	deployment := &appsv1.Deployment{}
	deploymentKey := types.NamespacedName{
		Name:      scaleTargetRef.Name,
		Namespace: cronhpa.Namespace,
	}

	// 获取 deployment
	if err := r.Get(ctx, deploymentKey, deployment); err != nil {
		if errors.IsNotFound(err) {
			log.Error(err, "Deployment not found", "deployment", deploymentKey)
		}
		return fmt.Errorf("failed to get deployment: %w", err)
	}

	// 检查当前副本数是否已经是目标副本数
	if deployment.Spec.Replicas != nil && *deployment.Spec.Replicas == job.Size {
		log.Info("Deployment already at desired replica count", "deployment", deploymentKey, "replicas", job.Size)
		return nil
	}

	// 更新副本数
	deployment.Spec.Replicas = &job.Size

	// 应用更新
	if err := r.Update(ctx, deployment); err != nil {
		return fmt.Errorf("failed to update deployment: %w", err)
	}

	log.Info("Successfully updated deployment replicas", "deployment", deploymentKey, "replicas", job.Size)

	return nil
}

// 获取下一个调度时间
func (r *HpaReconciler) getNextScheduledTime(schedule string, after time.Time) (time.Time, error) {
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	cronSchedule, err := parser.Parse(schedule)
	if err != nil {
		return time.Time{}, err
	}

	return cronSchedule.Next(after), nil
}
