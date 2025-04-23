package cmd

import (
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
)

func discoverGVR(config *rest.Config, resourceName string) (schema.GroupVersionResource, error) {
	disco, err := discovery.NewDiscoveryClientForConfig(config)
	if err != nil {
		return schema.GroupVersionResource{}, fmt.Errorf("could not create discovery client: %v", err)
	}

	apiGroupResources, err := restmapper.GetAPIGroupResources(disco)
	if err != nil {
		return schema.GroupVersionResource{}, fmt.Errorf("could not get API group resources: %v", err)
	}

	// Normalize the resource name to lowercase and singular
	resourceName = strings.ToLower(resourceName)

	// Try to find matching resource via shortcut
	if err != nil {
		// Fallback — manually scan for match
		for _, group := range apiGroupResources {
			for version, resList := range group.VersionedResources {
				for _, res := range resList {
					if res.Name == resourceName && !strings.Contains(res.Name, "/") {
						return schema.GroupVersionResource{
							Group:    group.Group.Name,
							Version:  version,
							Resource: res.Name,
						}, nil
					}
				}
			}
		}
	}

	return schema.GroupVersionResource{}, fmt.Errorf("resource '%s' not found in API discovery", resourceName)
}

