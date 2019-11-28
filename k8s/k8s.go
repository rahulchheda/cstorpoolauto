/*
Copyright 2019 The MayaData Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    https://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package k8s

import (
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// Kind is a custom datatype to refer to kubernetes native
// resource kind value
type Kind string

const (
	// NodeKind refers to kubernetes node's kind value
	NodeKind Kind = "Node"
)

// GetNestedSlice returns the slice found at given field path
// of the given object
func GetNestedSlice(obj *unstructured.Unstructured, fields ...string) ([]interface{}, error) {
	nestedSlice, found, err := unstructured.NestedSlice(obj.Object, fields...)
	if err != nil || !found {
		return nil, err
	}
	return nestedSlice, nil
}

// MergeNestedSlice merges the new value with the existing values at the
// given field path & returns the updated values
//
// TODO (@amitkumardas): Unit Tests
func MergeNestedSlice(obj *unstructured.Unstructured, new map[string]string, fields ...string) ([]interface{}, error) {
	nestedSlice, err := GetNestedSlice(obj, fields...)
	if err != nil {
		return nil, err
	}
	var indexKey string
	var indexValue string
	for k, v := range new {
		// One of these keys can be used to merge
		//
		// Note: There is no ordering with these keys. It is expected
		// that the provided new object to have only one of these keys.
		if k == "uid" || k == "id" || k == "name" {
			indexKey = k
			indexValue = v
			break
		}
	}
	var found bool
	var foundAt int
	for i, item := range nestedSlice {
		itemMap, ok := item.(map[string]string)
		if !ok {
			return nil, errors.Errorf("Invalid nested slice: Want map[string]string: Got %T", item)
		}
		for k, v := range itemMap {
			if k == indexKey && v == indexValue {
				found = true
				foundAt = i
				break
			}
		}
		if found {
			break
		}
	}
	if found {
		// replace with new item
		nestedSlice[foundAt] = new
	} else {
		// add the new item
		nestedSlice = append(nestedSlice, new)
	}
	return nestedSlice, nil
}

// MergeAndSetNestedSlice merges the provided conditions with existing
// ones if any against the provided object
func MergeAndSetNestedSlice(obj *unstructured.Unstructured, new map[string]string, fields ...string) ([]interface{}, error) {
	updatedSlice, err := MergeNestedSlice(obj, new, fields...)
	if err != nil {
		return nil, err
	}
	err = unstructured.SetNestedSlice(obj.Object, updatedSlice, fields...)
	if err != nil {
		return nil, err
	}
	return updatedSlice, nil
}

// MergeStatusConditions merges the provided conditions with existing
// ones if any & returns the updated conditions
//
// TODO (@amitkumardas): Unit Tests
func MergeStatusConditions(obj *unstructured.Unstructured, newCondition map[string]string) ([]interface{}, error) {
	return MergeNestedSlice(obj, newCondition, "status", "conditions")
}

// MergeAndSetStatusConditions merges the provided conditions with existing
// ones if any against the provided object
//
// TODO (@amitkumardas): Unit Tests
func MergeAndSetStatusConditions(obj *unstructured.Unstructured, newCondition map[string]string) ([]interface{}, error) {
	return MergeAndSetNestedSlice(obj, newCondition, "status", "conditions")
}

// GetNestedMap returns the map found at given field path of the given
// object
func GetNestedMap(obj *unstructured.Unstructured, fields ...string) (map[string]interface{}, error) {
	nestedMap, found, err := unstructured.NestedMap(obj.Object, fields...)
	if err != nil || !found {
		return nil, err
	}
	return nestedMap, nil
}

// GetNestedMapOrEmpty returns the map found at given field path
// of the given object. It returns empty map in case of error or
// if this map was not found.
func GetNestedMapOrEmpty(obj *unstructured.Unstructured, fields ...string) (map[string]interface{}, error) {
	nestedMap, err := GetNestedMap(obj, fields...)
	if nestedMap == nil {
		nestedMap = map[string]interface{}{}
	}
	return nestedMap, err
}