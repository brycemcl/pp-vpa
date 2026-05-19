/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

// Package scale documents the /scale subresource integration with the HPA.
//
// The /scale subresource is exposed declaratively by the CRD marker
//
//	// +kubebuilder:subresource:scale:specpath=.spec.targetReplicas,statuspath=.status.activeReplicas
//
// on PerPodVerticalPodAutoscaler. The Kubernetes API server then serves a
// virtual Scale subresource at
//
//	/apis/autoscaling.brycemclachlan.me/v1alpha1/namespaces/<ns>/perpodverticalpodautoscalers/<name>/scale
//
// HPA writes Scale.spec.replicas → .status.targetReplicas, and reads
// Scale.status.replicas ← .status.activeReplicas. The PP-VPA controller then
// reconciles the underlying Deployment to status.targetReplicas + status.temporaryReplicas
// (see PerPodVerticalPodAutoscalerReconciler.reconcileReplicas), isolating the
// HPA from temporary maxSurge inflation.
//
// No additional handler code is required at the controller layer; the marker
// on the CRD is the only piece. This file exists to make the integration
// discoverable in the repo layout.
package scale
