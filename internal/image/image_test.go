package image

import (
	"testing"

	"github.com/stretchr/testify/assert"
	kustomizetypes "sigs.k8s.io/kustomize/api/types"
)

func TestSplit(t *testing.T) {
	cases := []struct {
		name       string
		image      string
		wantName   string
		wantTag    string
		wantDigest string
	}{
		{
			name:     "image with tag",
			image:    "hello:v1.0",
			wantName: "hello",
			wantTag:  "v1.0",
		},
		{
			name:       "image with digest",
			image:      "hello@sha256:25a0d4",
			wantName:   "hello",
			wantDigest: "sha256:25a0d4",
		},
		{
			name:     "custom registry",
			image:    "foo.io/example/hello:v1.0",
			wantName: "foo.io/example/hello",
			wantTag:  "v1.0",
		},
		{
			name:     "no tag",
			image:    "hello",
			wantName: "hello",
		},
		{
			name:  "empty",
			image: "",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			gotName, gotTag, gotDigest := Split(tc.image)
			assert.Equal(t, tc.wantName, gotName)
			assert.Equal(t, tc.wantTag, gotTag)
			assert.Equal(t, tc.wantDigest, gotDigest)
		})
	}
}

func TestGetKustomizeImageList(t *testing.T) {
	cases := []struct {
		name         string
		imageName    string
		image        string
		defaultImage string
		wantImage    *kustomizetypes.Image
	}{
		{
			name:         "image found",
			imageName:    "foo",
			image:        "example.com/foo:v1.0",
			defaultImage: "example.com/foo:v0.0",
			wantImage: &kustomizetypes.Image{
				Name:    "foo",
				NewName: "example.com/foo",
				NewTag:  "v1.0",
				Digest:  "",
			},
		},
		{
			name:         "default image found",
			imageName:    "foo",
			image:        "",
			defaultImage: "example.com/foo:v0.0",
			wantImage: &kustomizetypes.Image{
				Name:    "foo",
				NewName: "example.com/foo",
				NewTag:  "v0.0",
				Digest:  "",
			},
		},
		{
			name:         "not found",
			imageName:    "foo",
			image:        "",
			defaultImage: "",
			wantImage:    nil,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			img := GetKustomizeImage(tc.imageName, tc.image, tc.defaultImage)
			assert.Equal(t, tc.wantImage, img)
		})
	}
}
