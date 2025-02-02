package ehpa

import (
	"context"
	"fmt"
	"time"

	autoscalingv2 "k8s.io/api/autoscaling/v2beta2"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	autoscalingapi "github.com/gocrane/api/autoscaling/v1alpha1"
	predictionapi "github.com/gocrane/api/prediction/v1alpha1"

	"github.com/gocrane/crane/pkg/known"
)

func (c *EffectiveHPAController) ReconcilePodPredication(ctx context.Context, ehpa *autoscalingapi.EffectiveHorizontalPodAutoscaler) (*predictionapi.PodGroupPrediction, error) {
	predictionList := &predictionapi.PodGroupPredictionList{}
	opts := []client.ListOption{
		client.MatchingLabels(map[string]string{known.EffectiveHorizontalPodAutoscalerUidLabel: string(ehpa.UID)}),
	}
	err := c.Client.List(ctx, predictionList, opts...)
	if err != nil {
		if errors.IsNotFound(err) {
			return c.CreatePodPrediction(ctx, ehpa)
		} else {
			c.Recorder.Event(ehpa, v1.EventTypeNormal, "FailedGetPrediction", err.Error())
			c.Log.Error(err, "Failed to get PodGroupPrediction", "ehpa", klog.KObj(ehpa))
			return nil, err
		}
	} else if len(predictionList.Items) == 0 {
		return c.CreatePodPrediction(ctx, ehpa)
	}

	return c.UpdatePodPredictionIfNeed(ctx, ehpa, &predictionList.Items[0])
}

func (c *EffectiveHPAController) GetPodPredication(ctx context.Context, ehpa *autoscalingapi.EffectiveHorizontalPodAutoscaler) (*predictionapi.PodGroupPrediction, error) {
	predictionList := &predictionapi.PodGroupPredictionList{}
	opts := []client.ListOption{
		client.MatchingLabels(map[string]string{known.EffectiveHorizontalPodAutoscalerUidLabel: string(ehpa.UID)}),
	}
	err := c.Client.List(ctx, predictionList, opts...)
	if err != nil {
		return nil, err
	} else if len(predictionList.Items) == 0 {
		return nil, nil
	}

	return &predictionList.Items[0], nil
}

func (c *EffectiveHPAController) CreatePodPrediction(ctx context.Context, ehpa *autoscalingapi.EffectiveHorizontalPodAutoscaler) (*predictionapi.PodGroupPrediction, error) {
	podPrediction, err := c.NewPodPredictionObject(ehpa)
	if err != nil {
		c.Recorder.Event(ehpa, v1.EventTypeNormal, "FailedCreatePredictionObject", err.Error())
		c.Log.Error(err, "Failed to create object", "PodGroupPrediction", podPrediction)
		return nil, err
	}

	err = c.Client.Create(ctx, podPrediction)
	if err != nil {
		c.Recorder.Event(ehpa, v1.EventTypeNormal, "FailedCreatePrediction", err.Error())
		c.Log.Error(err, "Failed to create", "PodGroupPrediction", podPrediction)
		return nil, err
	}

	c.Log.Info("Create successfully", "PodGroupPrediction", klog.KObj(podPrediction))
	c.Recorder.Event(ehpa, v1.EventTypeNormal, "PodGroupPredictionCreated", "Create PodGroupPrediction successfully")

	return podPrediction, nil
}

func (c *EffectiveHPAController) UpdatePodPredictionIfNeed(ctx context.Context, ehpa *autoscalingapi.EffectiveHorizontalPodAutoscaler, podPredictionExist *predictionapi.PodGroupPrediction) (*predictionapi.PodGroupPrediction, error) {
	podPrediction, err := c.NewPodPredictionObject(ehpa)
	if err != nil {
		c.Recorder.Event(ehpa, v1.EventTypeNormal, "FailedCreatePredictionObject", err.Error())
		c.Log.Error(err, "Failed to create object", "PodGroupPrediction", podPrediction)
		return nil, err
	}

	if !equality.Semantic.DeepEqual(&podPredictionExist.Spec, &podPrediction.Spec) {
		c.Log.V(4).Info("PodGroupPrediction is unsynced according to EffectiveHorizontalPodAutoscaler, should be updated", "currentPodPrediction", podPredictionExist.Spec, "expectPodPrediction", podPrediction.Spec)

		podPredictionExist.Spec = podPrediction.Spec
		err := c.Update(ctx, podPredictionExist)
		if err != nil {
			c.Recorder.Event(ehpa, v1.EventTypeNormal, "FailedUpdatePrediction", err.Error())
			c.Log.Error(err, "Failed to update", "PodGroupPrediction", podPredictionExist)
			return nil, err
		}

		c.Log.Info("Update PodGroupPrediction successful", "PodGroupPrediction", klog.KObj(podPredictionExist))
	}

	return podPredictionExist, nil
}

func (c *EffectiveHPAController) NewPodPredictionObject(ehpa *autoscalingapi.EffectiveHorizontalPodAutoscaler) (*predictionapi.PodGroupPrediction, error) {
	name := fmt.Sprintf("ehpa-%s", ehpa.Name)
	prediction := &predictionapi.PodGroupPrediction{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: ehpa.Namespace, // the same namespace to effectivehpa
			Name:      name,
			Labels: map[string]string{
				"app.kubernetes.io/name":                       name,
				"app.kubernetes.io/part-of":                    ehpa.Name,
				"app.kubernetes.io/managed-by":                 known.EffectiveHorizontalPodAutoscalerManagedBy,
				known.EffectiveHorizontalPodAutoscalerUidLabel: string(ehpa.UID),
			},
		},
		Spec: predictionapi.PodGroupPredictionSpec{
			PredictionWindow:        metav1.Duration{Duration: time.Duration(*ehpa.Spec.Prediction.PredictionWindowSeconds) * time.Second},
			Mode:                    predictionapi.PredictionModeRange,
			MetricPredictionConfigs: make([]predictionapi.MetricPredictionConfig, 0),
			WorkloadRef:             &ehpa.Spec.ScaleTargetRef,
		},
	}

	var metricPredictionConfigs []predictionapi.MetricPredictionConfig
	for _, metric := range ehpa.Spec.Metrics {
		// Convert resource metric into prediction metric
		if metric.Type == autoscalingv2.ResourceMetricSourceType {
			metricName, err := GetPredictionMetricName(metric.Resource.Name)
			if err != nil {
				return nil, err
			}

			metricPredictionConfigs = append(metricPredictionConfigs, predictionapi.MetricPredictionConfig{
				MetricName:    metricName,
				AlgorithmType: ehpa.Spec.Prediction.PredictionAlgorithm.AlgorithmType,
				DSP:           ehpa.Spec.Prediction.PredictionAlgorithm.DSP.DeepCopy(),
				Percentile:    ehpa.Spec.Prediction.PredictionAlgorithm.Percentile.DeepCopy(),
			})
		}
	}
	prediction.Spec.MetricPredictionConfigs = metricPredictionConfigs

	// EffectiveHPA control the underground prediction so set controller reference for it here
	if err := controllerutil.SetControllerReference(ehpa, prediction, c.Scheme); err != nil {
		return nil, err
	}

	return prediction, nil
}

func IsPredictionEnabled(ehpa *autoscalingapi.EffectiveHorizontalPodAutoscaler) bool {
	return ehpa.Spec.Prediction != nil && ehpa.Spec.Prediction.PredictionWindowSeconds != nil && ehpa.Spec.Prediction.PredictionAlgorithm != nil
}

func setPredictionCondition(status *autoscalingapi.EffectiveHorizontalPodAutoscalerStatus, conditions []predictionapi.PodGroupPredictionCondition) {
	for _, cond := range conditions {
		if cond.Type == predictionapi.PredictionConditionPredicting {
			if len(cond.Reason) > 0 && len(cond.Message) > 0 {
				setCondition(status, autoscalingapi.PredictionReady, cond.Status, cond.Reason, cond.Message)
			}
		}
	}
}
