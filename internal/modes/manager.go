/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

// Package modes wires the control-plane and node-agent run-modes.
package modes

import (
	"crypto/tls"
	"fmt"
	"os"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	autoscalingv1alpha1 "github.com/brycemclachlan/pp-vpa/api/v1alpha1"
	"github.com/brycemclachlan/pp-vpa/internal/controller"
	"github.com/brycemclachlan/pp-vpa/internal/eviction"
	webhookv1 "github.com/brycemclachlan/pp-vpa/internal/webhook/v1"
)

// ManagerOptions configures RunManager.
type ManagerOptions struct {
	MetricsAddr          string
	ProbeAddr            string
	WebhookCertDir       string
	EnableLeaderElection bool
	EnableWebhooks       bool
}

// RunManager runs the control-plane: controllers + webhook + recommender.
func RunManager(opts ManagerOptions) error {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(autoscalingv1alpha1.AddToScheme(scheme))

	tlsOpts := []func(*tls.Config){
		func(c *tls.Config) { c.NextProtos = []string{"http/1.1"} },
	}
	wopts := webhook.Options{TLSOpts: tlsOpts}
	if opts.WebhookCertDir != "" {
		wopts.CertDir = opts.WebhookCertDir
	}
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		Metrics:                metricsserver.Options{BindAddress: opts.MetricsAddr, TLSOpts: tlsOpts},
		WebhookServer:          webhook.NewServer(wopts),
		HealthProbeBindAddress: opts.ProbeAddr,
		LeaderElection:         opts.EnableLeaderElection,
		LeaderElectionID:       "ppvpa.brycemclachlan.me",
	})
	if err != nil {
		return fmt.Errorf("creating manager: %w", err)
	}

	if err := (&controller.PerPodVerticalPodAutoscalerReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme()}).SetupWithManager(mgr); err != nil {
		return fmt.Errorf("ppvpa controller: %w", err)
	}
	if err := (&controller.PodResourceRecommendationReconciler{
		Client:           mgr.GetClient(),
		Scheme:           mgr.GetScheme(),
		Anomaly:          &eviction.AnomalyHandler{Client: mgr.GetClient()},
		BudgetedFallback: &eviction.BudgetedHandler{Client: mgr.GetClient()},
	}).SetupWithManager(mgr); err != nil {
		return fmt.Errorf("prr controller: %w", err)
	}
	if err := (&controller.CheckpointReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme()}).SetupWithManager(mgr); err != nil {
		return fmt.Errorf("checkpoint controller: %w", err)
	}

	if opts.EnableWebhooks && os.Getenv("ENABLE_WEBHOOKS") != "false" {
		if err := webhookv1.SetupPodWebhookWithManager(mgr); err != nil {
			return fmt.Errorf("pod webhook: %w", err)
		}
	}

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		return err
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		return err
	}
	return mgr.Start(ctrl.SetupSignalHandler())
}
