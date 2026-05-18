package main

// renderCmuxHeaderPaneContent returns the string to display in the header
// pane for the given terminal dimensions. Below 60×16 it renders the
// minimum-size advisory; above threshold it returns the pane's normal content.
func renderCmuxHeaderPaneContent(w, h int) string {
	if cmuxPaneTooSmall(w, h) {
		return cmuxMinSizeAdvisory()
	}
	return ""
}

// renderCmuxLogPaneContent returns the string to display in the log pane for
// the given terminal dimensions. Below 60×16 it renders the minimum-size
// advisory; above threshold it returns the pane's normal content.
func renderCmuxLogPaneContent(w, h int) string {
	if cmuxPaneTooSmall(w, h) {
		return cmuxMinSizeAdvisory()
	}
	return ""
}

// renderCmuxOrchestratorPaneContent returns the string to display in the
// orchestrator pane for the given terminal dimensions. Below 60×16 it renders
// the minimum-size advisory; above threshold it returns the pane's normal
// content.
func renderCmuxOrchestratorPaneContent(w, h int) string {
	if cmuxPaneTooSmall(w, h) {
		return cmuxMinSizeAdvisory()
	}
	return ""
}
