package claude

import "errors"

const (
	AttachmentTypePlanMode          = "plan_mode"
	AttachmentTypePlanFileReference = "plan_file_reference"
	AttachmentTypePlanModeExit      = "plan_mode_exit"
	AttachmentTypePlanModeReentry   = "plan_mode_reentry"
)

type Attachment struct {
	Type         string `json:"type"`
	PlanFilePath string `json:"planFilePath"`
	PlanExists   bool   `json:"planExists"`
	PlanContent  string `json:"planContent"`
}

func (a *Attachment) Validate() error {
	if a == nil {
		return errors.New("claude attachment is nil")
	}
	if a.Type == "" {
		return errors.New("type must not be empty")
	}
	return nil
}
