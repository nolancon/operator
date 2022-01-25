package storageoscluster

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestGenerateSideCarContainerPatch(t *testing.T) {
	cases := []struct {
		name                   string
		nextIndex              int
		containerName          string
		image                  string
		config                 string
		readinessProbeInterval time.Duration
		wantPatch              string
	}{
		{
			name:                   "without context",
			nextIndex:              1,
			containerName:          "dummy-container",
			image:                  "dummy-image",
			readinessProbeInterval: time.Second,
			wantPatch:              "[{\"op\":\"add\",\"path\":\"/spec/template/spec/containers/1\",\"value\":{\"env\":[{\"name\":\"NODE_NAME\",\"valueFrom\":{\"fieldRef\":{\"apiVersion\":\"v1\",\"fieldPath\":\"spec.nodeName\"}}}],\"image\":\"dummy-image\",\"livenessProbe\":{\"httpGet\":{\"path\":\"/healthz\",\"port\":8081},\"initialDelaySeconds\":15,\"periodSeconds\":20},\"name\":\"dummy-container\",\"readinessProbe\":{\"httpGet\":{\"path\":\"/readyz\",\"port\":8081},\"initialDelaySeconds\":5,\"periodSeconds\":1}}}]",
		},
		{
			name:                   "with context",
			nextIndex:              1,
			containerName:          "dummy-container",
			image:                  "dummy-image",
			config:                 "foo= bar , bar=foo",
			readinessProbeInterval: time.Second,
			wantPatch:              "[{\"op\":\"add\",\"path\":\"/spec/template/spec/containers/1\",\"value\":{\"env\":[{\"name\":\"foo\",\"value\":\"bar\"},{\"name\":\"bar\",\"value\":\"foo\"},{\"name\":\"NODE_NAME\",\"valueFrom\":{\"fieldRef\":{\"apiVersion\":\"v1\",\"fieldPath\":\"spec.nodeName\"}}}],\"image\":\"dummy-image\",\"livenessProbe\":{\"httpGet\":{\"path\":\"/healthz\",\"port\":8081},\"initialDelaySeconds\":15,\"periodSeconds\":20},\"name\":\"dummy-container\",\"readinessProbe\":{\"httpGet\":{\"path\":\"/readyz\",\"port\":8081},\"initialDelaySeconds\":5,\"periodSeconds\":1}}}]",
		},
		{
			name:                   "with various context",
			nextIndex:              1,
			containerName:          "dummy-container",
			image:                  "dummy-image",
			config:                 "foo=,bar",
			readinessProbeInterval: time.Second,
			wantPatch:              "[{\"op\":\"add\",\"path\":\"/spec/template/spec/containers/1\",\"value\":{\"env\":[{\"name\":\"foo\",\"value\":\"\"},{\"name\":\"bar\",\"value\":\"\"},{\"name\":\"NODE_NAME\",\"valueFrom\":{\"fieldRef\":{\"apiVersion\":\"v1\",\"fieldPath\":\"spec.nodeName\"}}}],\"image\":\"dummy-image\",\"livenessProbe\":{\"httpGet\":{\"path\":\"/healthz\",\"port\":8081},\"initialDelaySeconds\":15,\"periodSeconds\":20},\"name\":\"dummy-container\",\"readinessProbe\":{\"httpGet\":{\"path\":\"/readyz\",\"port\":8081},\"initialDelaySeconds\":5,\"periodSeconds\":1}}}]",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			patch := generateSideCarContainerPatch(tc.nextIndex, tc.containerName, tc.image, tc.config, tc.readinessProbeInterval)

			var patchObj interface{}
			err := json.Unmarshal([]byte(patch.Patch), &patchObj)
			assert.Nil(t, err)

			cleanPatch, err := json.Marshal(patchObj)
			assert.Nil(t, err)

			assert.Equal(t, tc.wantPatch, string(cleanPatch))
		})
	}
}
