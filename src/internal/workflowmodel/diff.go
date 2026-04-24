package workflowmodel

import "reflect"

// IsDirty reports whether memory differs from the disk snapshot.
// Differences in UnknownFields are ignored — they are recorded on load and
// not written back on save, so they cannot contribute to dirty state.
func IsDirty(disk, memory WorkflowDoc) bool {
	disk.UnknownFields = nil
	memory.UnknownFields = nil
	return !reflect.DeepEqual(disk, memory)
}
