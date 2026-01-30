package sync

import (
	"fmt"
	"reflect"

	"github.com/syncflow/sync-engine/pkg/models"
)

// Calculator handles differential sync calculations
type Calculator struct{}

// NewCalculator creates a new differential sync calculator
func NewCalculator() *Calculator {
	return &Calculator{}
}

// CalculateDiff compares two documents and returns the differences
// oldDoc: previous version of the document (can be nil for inserts)
// newDoc: current version of the document (can be nil for deletes)
func (c *Calculator) CalculateDiff(oldDoc, newDoc map[string]interface{}) *models.DiffResult {
	result := &models.DiffResult{
		HasChanges:     false,
		AddedFields:    make(map[string]interface{}),
		ModifiedFields: make(map[string]interface{}),
		RemovedFields:  []string{},
	}

	// Handle delete operation (newDoc is nil)
	if newDoc == nil {
		result.HasChanges = true
		return result
	}

	// Handle insert operation (oldDoc is nil)
	if oldDoc == nil {
		result.HasChanges = true
		result.AddedFields = newDoc
		return result
	}

	// Compare fields in newDoc
	for key, newValue := range newDoc {
		// Skip internal MongoDB fields
		if shouldSkipField(key) {
			continue
		}

		oldValue, existsInOld := oldDoc[key]

		if !existsInOld {
			// Field was added
			result.AddedFields[key] = newValue
			result.HasChanges = true
		} else if !deepEqual(oldValue, newValue) {
			// Field was modified
			result.ModifiedFields[key] = newValue
			result.HasChanges = true
		}
	}

	// Check for removed fields
	for key := range oldDoc {
		if shouldSkipField(key) {
			continue
		}

		if _, existsInNew := newDoc[key]; !existsInNew {
			result.RemovedFields = append(result.RemovedFields, key)
			result.HasChanges = true
		}
	}

	return result
}

// deepEqual performs deep equality check for values
func deepEqual(a, b interface{}) bool {
	return reflect.DeepEqual(a, b)
}

// shouldSkipField determines if a field should be skipped in diff calculation
func shouldSkipField(fieldName string) bool {
	// Skip MongoDB internal fields
	skipFields := map[string]bool{
		"_id": true, // MongoDB ObjectID (never changes)
	}

	return skipFields[fieldName]
}

// MergeChanges applies changes to a base document
func (c *Calculator) MergeChanges(baseDoc map[string]interface{}, changes *models.DiffResult) map[string]interface{} {
	if baseDoc == nil {
		baseDoc = make(map[string]interface{})
	}

	// Create a copy to avoid mutating the original
	result := copyMap(baseDoc)

	// Apply added fields
	for key, value := range changes.AddedFields {
		result[key] = value
	}

	// Apply modified fields
	for key, value := range changes.ModifiedFields {
		result[key] = value
	}

	// Remove deleted fields
	for _, key := range changes.RemovedFields {
		delete(result, key)
	}

	return result
}

// copyMap creates a deep copy of a map
func copyMap(original map[string]interface{}) map[string]interface{} {
	copy := make(map[string]interface{})
	for key, value := range original {
		copy[key] = deepCopy(value)
	}
	return copy
}

// deepCopy creates a deep copy of a value
func deepCopy(value interface{}) interface{} {
	if value == nil {
		return nil
	}

	switch v := value.(type) {
	case map[string]interface{}:
		return copyMap(v)
	case []interface{}:
		copy := make([]interface{}, len(v))
		for i, item := range v {
			copy[i] = deepCopy(item)
		}
		return copy
	default:
		// Primitive types are copied by value
		return v
	}
}

// CalculatePatch creates a patch that can be applied to transform oldDoc into newDoc
func (c *Calculator) CalculatePatch(oldDoc, newDoc map[string]interface{}) map[string]interface{} {
	diff := c.CalculateDiff(oldDoc, newDoc)

	patch := make(map[string]interface{})

	if len(diff.AddedFields) > 0 {
		patch["$set"] = mergeMaps(diff.AddedFields, diff.ModifiedFields)
	} else if len(diff.ModifiedFields) > 0 {
		patch["$set"] = diff.ModifiedFields
	}

	if len(diff.RemovedFields) > 0 {
		unset := make(map[string]interface{})
		for _, field := range diff.RemovedFields {
			unset[field] = ""
		}
		patch["$unset"] = unset
	}

	return patch
}

// mergeMaps merges multiple maps into one
func mergeMaps(maps ...map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})
	for _, m := range maps {
		for k, v := range m {
			result[k] = v
		}
	}
	return result
}

// GetChangedFieldNames returns a list of all changed field names
func (c *Calculator) GetChangedFieldNames(diff *models.DiffResult) []string {
	var fieldNames []string

	for key := range diff.AddedFields {
		fieldNames = append(fieldNames, key)
	}

	for key := range diff.ModifiedFields {
		fieldNames = append(fieldNames, key)
	}

	fieldNames = append(fieldNames, diff.RemovedFields...)

	return fieldNames
}

// FormatDiffSummary creates a human-readable summary of changes
func (c *Calculator) FormatDiffSummary(diff *models.DiffResult) string {
	if !diff.HasChanges {
		return "No changes detected"
	}

	return fmt.Sprintf("Changes: %d added, %d modified, %d removed",
		len(diff.AddedFields),
		len(diff.ModifiedFields),
		len(diff.RemovedFields))
}
