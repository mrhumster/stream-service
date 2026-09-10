package models

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
)

func (s *Stream) IsDraft() bool {
	return s.Status == StatusDraft
}

func (s *Stream) IsProcessing() bool {
	return s.Status == StatusProcessing
}

func (s *Stream) IsPublished() bool {
	return s.Status == StatusPublished
}

func (s *Stream) CanEdit() bool {
	return s.Status == StatusDraft || s.Status == StatusReady
}

func (s *Stream) CanPublish() bool {
	return s.Status == StatusReady
}

func (s *Stream) Publish() error {
	if !s.CanPublish() {
		return fmt.Errorf("cannot publish stream in status: %s", s.Status)
	}
	s.Status = StatusPublished
	now := time.Now()
	s.PublishedAt = &now
	return nil
}

func (s *Stream) Unpublish() {
	s.Status = StatusReady
	s.PublishedAt = nil
}

func (s *Stream) SetStorageInfo(storageInfo *StreamStorage) error {
	data, err := json.Marshal(storageInfo)
	if err != nil {
		return err
	}
	s.Storage = datatypes.JSON(data)
	return nil
}

func (s *Stream) GetStorageInfo() (*StreamStorage, error) {
	var storageInfo StreamStorage
	if err := json.Unmarshal(s.Storage, &storageInfo); err != nil {
		return nil, err
	}
	return &storageInfo, nil
}

func (s *Stream) GetMetadata() (*StreamMetadata, error) {
	var metadata StreamMetadata
	if err := json.Unmarshal(s.Metadata, &metadata); err != nil {
		return nil, err
	}
	return &metadata, nil
}

func (s *Stream) SetMetadata(metadata *StreamMetadata) error {
	data, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	s.Metadata = datatypes.JSON(data)
	return nil
}

func (s *Stream) GetAnalitics() (*StreamAnalytics, error) {
	var analytics StreamAnalytics
	if err := json.Unmarshal(s.Analytics, &analytics); err != nil {
		return nil, err
	}
	return &analytics, nil
}

func (s *Stream) SetAnalitics(analitics *StreamAnalytics) error {
	data, err := json.Marshal(analitics)
	if err != nil {
		return err
	}
	s.Analytics = datatypes.JSON(data)
	return nil
}

func (s *Stream) UpdateProcessing(process int, steps []string, errMsg *string, taskID *string) error {
	return s.SetTaskProgress(TaskTypeTranscode, process, steps, errMsg, taskID)
}

func (s *Stream) ProcessingTasks() ([]StreamProcessingTask, error) {
	if len(s.Processing) == 0 || string(s.Processing) == "null" {
		return []StreamProcessingTask{}, nil
	}
	var tasks []StreamProcessingTask
	if err := json.Unmarshal(s.Processing, &tasks); err != nil {
		return nil, err
	}
	return tasks, nil
}

func (s *Stream) SetTaskProgress(taskType string, progress int, steps []string, errMsg, taskID *string) error {
	tasks, err := s.ProcessingTasks()
	if err != nil {
		return err
	}

	updated := false
	for i := range tasks {
		if tasks[i].TaskType == taskType {
			tasks[i].Progress = progress
			tasks[i].Steps = steps
			tasks[i].Error = errMsg
			tasks[i].TaskID = taskID
			updated = true
			break
		}
	}
	if !updated {
		tasks = append(tasks, StreamProcessingTask{
			TaskType: taskType,
			Progress: progress,
			Steps:    steps,
			Error:    errMsg,
			TaskID:   taskID,
		})
	}

	data, err := json.Marshal(tasks)
	if err != nil {
		return err
	}
	s.Processing = datatypes.JSON(data)
	return nil
}

func (s *Stream) SetInitialTasks(tasks []StreamProcessingTask) error {
	data, err := json.Marshal(tasks)
	if err != nil {
		return err
	}
	s.Processing = datatypes.JSON(data)
	return nil
}

func (s *Stream) IsPublic() bool {
	return s.Visibility == VisibilityPublic
}

func (s *Stream) IsOwnedBy(userID uuid.UUID) bool {
	return s.OwnerID == userID
}
