package workflowmodel

import "reflect"

// IsDirty reports whether memory differs from the disk snapshot.
func IsDirty(disk, memory WorkflowDoc) bool {
	return !reflect.DeepEqual(disk, memory)
}
