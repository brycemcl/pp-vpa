/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

package histogram

import (
	"bytes"
	"encoding/base64"
	"encoding/gob"
	"time"
)

// checkpoint is the on-disk representation written to PRR.status.histogramCheckpoint.
type checkpoint struct {
	MaxValue        float64
	FirstBucketSize float64
	BucketRatio     float64
	HalfLifeNS      int64
	Epsilon         float64
	Buckets         []float64
	TotalWeight     float64
	ReferenceUnixNS int64
}

// Encode serializes h into a base64+gob string suitable for a CRD status field.
func Encode(h *Histogram) (string, error) {
	cp := checkpoint{
		MaxValue:        h.opts.MaxValue,
		FirstBucketSize: h.opts.FirstBucketSize,
		BucketRatio:     h.opts.BucketRatio,
		HalfLifeNS:      h.opts.HalfLife.Nanoseconds(),
		Epsilon:         h.opts.Epsilon,
		Buckets:         h.Buckets(),
		TotalWeight:     h.totalWeight,
		ReferenceUnixNS: h.referenceTime.UnixNano(),
	}
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(&cp); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}

// Decode reconstructs a Histogram from a base64+gob payload.
func Decode(s string) (*Histogram, error) {
	raw, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return nil, err
	}
	var cp checkpoint
	if err := gob.NewDecoder(bytes.NewReader(raw)).Decode(&cp); err != nil {
		return nil, err
	}
	h, err := New(Options{
		MaxValue:        cp.MaxValue,
		FirstBucketSize: cp.FirstBucketSize,
		BucketRatio:     cp.BucketRatio,
		HalfLife:        time.Duration(cp.HalfLifeNS),
		Epsilon:         cp.Epsilon,
	})
	if err != nil {
		return nil, err
	}
	ref := time.Unix(0, cp.ReferenceUnixNS)
	if err := h.LoadBuckets(cp.Buckets, cp.TotalWeight, ref); err != nil {
		return nil, err
	}
	return h, nil
}
