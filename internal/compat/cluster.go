/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

package compat

import (
	"context"
	"strconv"
	"strings"

	"k8s.io/apimachinery/pkg/version"
	"k8s.io/client-go/discovery"
)

// ClusterCapabilities reflects API-server-side capability flags.
type ClusterCapabilities struct {
	KubeletMinor       int
	PodLevelResources  bool // requires v1.36+
	ContainerLevelOnly bool // true on v1.33–1.35
}

// ProbeCluster issues a Discovery /version call and derives capability flags.
func ProbeCluster(ctx context.Context, d discovery.DiscoveryInterface) (ClusterCapabilities, error) {
	v, err := d.ServerVersion()
	if err != nil {
		return ClusterCapabilities{}, err
	}
	return capsFromVersion(v), nil
}

func capsFromVersion(v *version.Info) ClusterCapabilities {
	minorStr := strings.TrimSuffix(v.Minor, "+")
	minor, _ := strconv.Atoi(minorStr)
	caps := ClusterCapabilities{KubeletMinor: minor}
	switch {
	case minor >= 36:
		caps.PodLevelResources = true
	case minor >= 33:
		caps.ContainerLevelOnly = true
	default:
		// older — neither supported
	}
	return caps
}
