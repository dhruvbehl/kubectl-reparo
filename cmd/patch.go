package cmd

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
        "github.com/evanphx/json-patch"
)

// setField applies dot-notation updates to an Unstructured object
func setFieldsOnObject(obj *unstructured.Unstructured, updates map[string]string) error {
	for path, val := range updates {
		if err := unstructured.SetNestedField(obj.Object, parseValue(val), strings.Split(path, ".")...); err != nil {
			return fmt.Errorf("could not set field %s: %w", path, err)
		}
	}
	return nil
}

func parseValue(val string) interface{} {
	if i, err := strconv.ParseInt(val, 10, 64); err == nil {
		return i
	}
	if val == "true" {
		return true
	}
	if val == "false" {
		return false
	}
	if val == "null" {
		return nil
	}
	return val
}

func parseSetFields(setStr string) map[string]string {
	result := map[string]string{}
	pairs := strings.Split(setStr, ",")
	for _, pair := range pairs {
		parts := strings.SplitN(pair, "=", 2)
		if len(parts) == 2 {
			result[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
		}
	}
	return result
}

// createPatchJSON returns the minimal patch to apply
func createPatchJSON(original, modified *unstructured.Unstructured) ([]byte, error) {
        origJSON, err := json.Marshal(original.Object)
        if err != nil {
                return nil, err
        }
        modJSON, err := json.Marshal(modified.Object)
        if err != nil {
                return nil, err
        }

        patch, err := jsonpatch.CreateMergePatch(origJSON, modJSON)
        if err != nil {
                return nil, err
        }

        return patch, nil
}

