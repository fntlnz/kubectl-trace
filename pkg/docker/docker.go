package docker

import (
	"fmt"
	"strings"
)

type Image struct {
	Hostname   string
	Repository string
	Name       string
	Tag        string
}

func ParseImageName(imageName string) (*Image, error) {
	parts := strings.Split(imageName, "/")
	tag := ""

	nameTag := strings.Split(parts[len(parts)-1], ":")
	parts[len(parts)-1] = nameTag[0]

	switch len(nameTag) {
	case 1:
	case 2:
		tag = nameTag[1]
	default:
		return nil, fmt.Errorf("Invalid docker image name '%s'; expected hostname/repository/name[:tag]", imageName)
	}

	switch len(parts) {
	case 3:
		return &Image{
			Hostname:   parts[0],
			Repository: parts[1],
			Name:       parts[2],
			Tag:        tag,
		}, nil
	case 2:
		return &Image{
			Repository: parts[0],
			Name:       parts[1],
			Tag:        tag,
		}, nil
	case 1:
		return &Image{
			Name: parts[0],
			Tag:  tag,
		}, nil
	default:
		return nil, fmt.Errorf("Invalid docker image name '%s'; expected hostname/repository/name[:tag]", imageName)
	}
}
