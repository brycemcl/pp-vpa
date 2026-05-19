/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

package patcher

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

func patchOpts() metav1.PatchOptions { return metav1.PatchOptions{FieldManager: "pp-vpa-node-agent"} }
