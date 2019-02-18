package main

import (
	"encoding/json"
	"os"
)

type restoredIssue struct {
	URL    string
	Owner  string
	Repo   string
	IssNum int
	Done   int
	Extra  string
}

func saveState(f *os.File, chkpt restoredIssue) error {

	data, err := json.Marshal(chkpt)
	if err != nil {
		return err
	}

	if err := os.Truncate(chkptLog, 0); err != nil {
		return err
	}

	if _, err := f.WriteAt(data, 0); err != nil {
		return err
	}

	if err := f.Sync(); err != nil {
		return err
	}

	return nil
}
