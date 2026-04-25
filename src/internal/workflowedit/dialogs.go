package workflowedit

// DialogKind identifies the currently-active modal dialog. At most one dialog
// slot is active at a time (D-8). Opening a second dialog replaces the first.
type DialogKind int

const (
	DialogNone                  DialogKind = iota // no dialog active
	DialogPathPicker                              // File > Open path input
	DialogNewChoice                               // File > New: Copy / Empty / Cancel
	DialogUnsavedChanges                          // Ctrl+Q with unsaved: Save / Cancel / Discard
	DialogQuitConfirm                             // Ctrl+Q without unsaved: Yes / No
	DialogExternalEditorOpening                   // external editor launching
	DialogFindingsPanel                           // validation findings (help may layer over this)
	DialogError                                   // generic error modal
	DialogCrashTempNotice                         // crash-era temp files detected at open
	DialogFirstSaveConfirm                        // first save to external or symlinked workflow
	DialogRemoveConfirm                           // Del step confirmation
	DialogFileConflict                            // mtime-mismatch: overwrite / reload / cancel
	DialogSaveInProgress                          // save running while quit was requested
	DialogRecovery                                // malformed config.json recovery view
	DialogAcknowledgeFindings                     // warn/info-only findings: proceed or cancel
	DialogCopyBrokenRef                           // broken default model reference: copy anyway / cancel
)

// dialogState holds the active dialog's kind and any kind-specific payload.
type dialogState struct {
	kind    DialogKind
	payload any
}
