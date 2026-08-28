package loki

// SetLabels replaces the whole label set of the logger.
func (l *Logger) SetLabels(labels map[string]string) {
	newLabels := copyLabels(labels)

	l.labelsMu.Lock()
	defer l.labelsMu.Unlock()

	l.labels = newLabels
}

// Labels returns a copy of the current label set.
func (l *Logger) Labels() map[string]string {
	return copyLabels(l.currentLabels())
}

// currentLabels returns the label set as is. The returned map must be treated as
// read-only: it is shared between concurrent log calls.
func (l *Logger) currentLabels() map[string]string {
	l.labelsMu.RLock()
	defer l.labelsMu.RUnlock()

	return l.labels
}

func copyLabels(labels map[string]string) map[string]string {
	result := make(map[string]string, len(labels))
	for k, v := range labels {
		result[k] = v
	}

	return result
}
