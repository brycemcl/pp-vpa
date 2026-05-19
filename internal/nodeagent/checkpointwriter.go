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

package nodeagent

import (
	"context"
	"time"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	autoscalingv1alpha1 "github.com/brycemclachlan/pp-vpa/api/v1alpha1"
	"github.com/brycemclachlan/pp-vpa/internal/recommender/histogram"
)

// CheckpointWriter periodically encodes per-pod histograms into the matching
// PRR's status.histogramCheckpoint.
type CheckpointWriter struct {
	Client   client.Client
	Interval time.Duration
}

// Write encodes h into the named PRR.
func (w *CheckpointWriter) Write(ctx context.Context, ns, prrName string, h *histogram.Histogram) error {
	enc, err := histogram.Encode(h)
	if err != nil {
		return err
	}
	var prr autoscalingv1alpha1.PodResourceRecommendation
	if err := w.Client.Get(ctx, types.NamespacedName{Namespace: ns, Name: prrName}, &prr); err != nil {
		return err
	}
	patch := prr.DeepCopy()
	patch.Status.HistogramCheckpoint = enc
	return w.Client.Status().Patch(ctx, patch, client.MergeFrom(&prr))
}
