package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/bitrise-io/go-utils/log"
)

type restoredIssue struct {
	URL    string
	Done   int
}

func saveState(f *os.File, chkpt restoredIssue) error {

	data, err := json.Marshal(chkpt)
	if err != nil {
		return fmt.Errorf("unmarshal: %s", err)
	}

	if err := os.Truncate(chkptLog, 0); err != nil {
		return fmt.Errorf("truncate checkpoint file: %s", err)
	}

	log.Debugf("writing data to checkpoint file: %s", string(data))
	if _, err := f.WriteAt(data, 0); err != nil {
		return fmt.Errorf("write to checkpoint file: %s", err)
	}

	if err := f.Sync(); err != nil {
		return fmt.Errorf("sync changes to checkpoint file: %s", err)
	}

	return nil
}
