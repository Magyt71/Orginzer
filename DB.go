package main

import (
	"encoding/json"
	"os"
)

type DiskConfig struct {
	WatchPaths      []string          `json:"watch_paths"`
	TargetFolder    map[string]string `json:"target_folder"`
	DestinationPath string            `json:"destination_path"`
	Notifications   bool              `json:"notifications"`
	RunAtStartup    bool              `json:"run_at_startup"`
	RecentMoves     []MoveRecord      `json:"recent_moves"`
}

func (O *Organizer) SaveConfigToDisk() error {
	O.Config.mu.Lock()
	O.Config.Rmu.RLock()

	// Create a safe deep copy of RecentMoves to avoid data races during JSON serialization
	safeMoves := make([]MoveRecord, len(O.RecentMoves))
	copy(safeMoves, O.RecentMoves)

	dataTosave := DiskConfig{
		WatchPaths:      O.Config.WatchPaths,
		TargetFolder:    O.Config.TargetMap,
		DestinationPath: O.Config.DestinationPath,
		Notifications:   O.Config.Notifications,
		RunAtStartup:    O.Config.RunAtStartup,
		RecentMoves:     safeMoves,
	}
	O.Config.Rmu.RUnlock()
	O.Config.mu.Unlock()

	fileBytes, err := json.MarshalIndent(dataTosave, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile("Settings.json", fileBytes, 0644)
}

func LoadConfigFromDisk() (*DiskConfig, error) {
	fileBytes, err := os.ReadFile("Settings.json")
	if err != nil {
		return nil, err
	}
	var savedData DiskConfig
	err = json.Unmarshal(fileBytes, &savedData)
	if err != nil {
		return nil, err
	}
	return &savedData, nil
}
