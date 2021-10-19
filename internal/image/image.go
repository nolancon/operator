package image

import (
	kustomizeimage "sigs.k8s.io/kustomize/api/image"
	kustomizetypes "sigs.k8s.io/kustomize/api/types"
)

// NamedImages is a map of image name and image address/value.
type NamedImages map[string]string

// Split splits the given image into name and tag, without the tag separator.
func Split(image string) (name, tag, digest string) {
	if image == "" {
		return
	}

	nameStr, tagStr := kustomizeimage.Split(image)

	if tagStr != "" {
		// Use the separator identify tag and digest.
		separator := tagStr[:1]

		// kustomize image Split returns tag with the separator. Trim the separator
		// from the tag.
		val := tagStr[1:]

		switch separator {
		case ":":
			return nameStr, val, ""
		case "@":
			return nameStr, "", val
		}
	}
	return nameStr, "", ""
}

// GetKustomizeImage takes a name, image and default image and returns a kustomize Image.
func GetKustomizeImage(imageName, image, defaultImage string) *kustomizetypes.Image {
	if image == "" {
		image = defaultImage
	}
	if image == "" {
		return nil
	}

	name, tag, digest := Split(image)

	return &kustomizetypes.Image{
		Name:    imageName,
		NewName: name,
		NewTag:  tag,
		Digest:  digest,
	}
}
