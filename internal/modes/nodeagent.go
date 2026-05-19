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

package modes

import (
	"context"
	"fmt"
	"os"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	autoscalingv1alpha1 "github.com/brycemclachlan/pp-vpa/api/v1alpha1"
	"github.com/brycemclachlan/pp-vpa/internal/nodeagent"
)

// NodeAgentOptions configures RunNodeAgent.
type NodeAgentOptions struct {
	NodeName      string
	CgroupRoot    string
	KubeletConfig string
}

// RunNodeAgent boots the DaemonSet code path.
func RunNodeAgent(opts NodeAgentOptions) error {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(autoscalingv1alpha1.AddToScheme(scheme))

	cfg := ctrl.GetConfigOrDie()
	kc, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return fmt.Errorf("kube client: %w", err)
	}
	cc, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		return fmt.Errorf("controller-runtime client: %w", err)
	}
	nodeName := opts.NodeName
	if nodeName == "" {
		nodeName = os.Getenv("NODE_NAME")
	}
	if nodeName == "" {
		return fmt.Errorf("--node-name or NODE_NAME env required")
	}
	a, err := nodeagent.New(nodeagent.Config{
		NodeName:      nodeName,
		CgroupRoot:    opts.CgroupRoot,
		KubeletConfig: opts.KubeletConfig,
	}, cc, kc)
	if err != nil {
		return fmt.Errorf("agent: %w", err)
	}

	// Resolve node allocatable for priority scoring.
	var nodeObj corev1.Node
	if err := cc.Get(nil, client.ObjectKey{Name: nodeName}, &nodeObj); err != nil {
		return fmt.Errorf("get node %s: %w", nodeName, err)
	}

	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme: scheme,
	})
	if err != nil {
		return fmt.Errorf("creating manager: %w", err)
	}

	if err := (&nodeagent.PodReconciler{
		Client:   mgr.GetClient(),
		Scheme:   scheme,
		Agent:    a,
		NodeName: nodeName,
	}).SetupWithManager(mgr); err != nil {
		return fmt.Errorf("pod reconciler: %w", err)
	}

	// Run the agent's sampling loop alongside the manager.
	if err := mgr.Add(manager.RunnableFunc(func(ctx context.Context) error {
		return a.Run(ctx)
	})); err != nil {
		return fmt.Errorf("add agent runnable: %w", err)
	}

	ctx := ctrl.SetupSignalHandler()
	return mgr.Start(ctx)
}
