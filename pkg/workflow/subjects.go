package workflow

import (
	"fmt"
	"strings"
)

const DefaultSubjectPrefix = "workflow"

func subjectPrefix(prefix string) string {
	prefix = strings.Trim(prefix, ".")
	if prefix == "" {
		return DefaultSubjectPrefix
	}
	return prefix
}

func SubmitSubject(prefix string) string {
	return subjectPrefix(prefix) + ".submit"
}

func ResultSubject(prefix, taskID string) string {
	return fmt.Sprintf("%s.result.%s", subjectPrefix(prefix), taskID)
}

func ResultFilterSubject(prefix string) string {
	return subjectPrefix(prefix) + ".result.>"
}

func CancelSubject(prefix string) string {
	return subjectPrefix(prefix) + ".cancel"
}

func WorkflowCancelSubject(prefix, workflowID string) string {
	return fmt.Sprintf("%s.cancel.%s", subjectPrefix(prefix), workflowID)
}
