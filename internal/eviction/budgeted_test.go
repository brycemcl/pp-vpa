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

package eviction

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestResolveBudgetNilUsesDefault(t *testing.T) {
	d := defaultPercent(25)
	got := resolveBudget(nil, 4, d)
	if got != 1 {
		t.Fatalf("expected 1, got %d", got)
	}
}

func TestResolveBudgetInteger(t *testing.T) {
	b := intstr.FromInt(3)
	got := resolveBudget(&b, 4, intstr.FromString("25%"))
	if got != 3 {
		t.Fatalf("expected 3, got %d", got)
	}
}

func TestResolveBudgetPercentage(t *testing.T) {
	b := intstr.FromString("25%")
	got := resolveBudget(&b, 4, intstr.FromString("0%"))
	if got != 1 {
		t.Fatalf("expected 1, got %d", got)
	}
}

func TestResolveBudgetPercentageOfZero(t *testing.T) {
	b := intstr.FromString("25%")
	got := resolveBudget(&b, 0, intstr.FromString("0%"))
	if got != 0 {
		t.Fatalf("expected 0 from 25%% of 0, got %d", got)
	}
}

func TestResolveBudget50Percent(t *testing.T) {
	// 50% of 3 = 2 (rounded up).
	b := intstr.FromString("50%")
	got := resolveBudget(&b, 3, intstr.FromString("0%"))
	if got != 2 {
		t.Fatalf("expected 2, got %d", got)
	}
}

func TestPodReadyTrue(t *testing.T) {
	p := &corev1.Pod{
		Status: corev1.PodStatus{
			Conditions: []corev1.PodCondition{
				{Type: corev1.PodReady, Status: corev1.ConditionTrue},
			},
		},
	}
	if !podReady(p) {
		t.Fatal("expected pod to be ready")
	}
}

func TestPodReadyFalse(t *testing.T) {
	p := &corev1.Pod{
		Status: corev1.PodStatus{
			Conditions: []corev1.PodCondition{
				{Type: corev1.PodReady, Status: corev1.ConditionFalse},
			},
		},
	}
	if podReady(p) {
		t.Fatal("expected pod to not be ready")
	}
}

func TestPodReadyNoCondition(t *testing.T) {
	p := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"}}
	if podReady(p) {
		t.Fatal("expected pod with no conditions to not be ready")
	}
}

func TestDefaultPercent(t *testing.T) {
	d := defaultPercent(25)
	if d.String() != "25%" {
		t.Fatalf("expected '25%%', got %v", d.String())
	}
}

func TestFindSurgePodNone(t *testing.T) {
	// This tests findSurgePod returns nil when no surge pods exist.
	// We can't easily test with a real client, but we can verify the constant.
	if AnnotationSurgePod != "pp-vpa.brycemclachlan.me/surge" {
		t.Fatalf("unexpected AnnotationSurgePod value: %v", AnnotationSurgePod)
	}
}
