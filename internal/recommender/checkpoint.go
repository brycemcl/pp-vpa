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

package recommender

import (
	"bytes"
	"encoding/base64"
	"encoding/gob"
	"fmt"
	"time"

	"github.com/brycemclachlan/pp-vpa/internal/recommender/histogram"
)

// workloadCheckpoint is the on-disk form of WorkloadHistograms.
type workloadCheckpoint struct {
	CPU    string
	Memory string
	Time   int64
}

// EncodeWorkload serializes the workload histograms into a single base64+gob
// payload suitable for PerPodVerticalPodAutoscalerCheckpoint.status.
func EncodeWorkload(wh *WorkloadHistograms, now time.Time) (string, error) {
	cpu, err := histogram.Encode(wh.CPU)
	if err != nil {
		return "", fmt.Errorf("encode cpu: %w", err)
	}
	mem, err := histogram.Encode(wh.Memory)
	if err != nil {
		return "", fmt.Errorf("encode memory: %w", err)
	}
	cp := workloadCheckpoint{CPU: cpu, Memory: mem, Time: now.UnixNano()}
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(&cp); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}

// DecodeWorkload restores WorkloadHistograms from the encoded form.
func DecodeWorkload(s string) (*WorkloadHistograms, error) {
	raw, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return nil, err
	}
	var cp workloadCheckpoint
	if err := gob.NewDecoder(bytes.NewReader(raw)).Decode(&cp); err != nil {
		return nil, err
	}
	cpu, err := histogram.Decode(cp.CPU)
	if err != nil {
		return nil, fmt.Errorf("decode cpu: %w", err)
	}
	mem, err := histogram.Decode(cp.Memory)
	if err != nil {
		return nil, fmt.Errorf("decode memory: %w", err)
	}
	return &WorkloadHistograms{CPU: cpu, Memory: mem}, nil
}
