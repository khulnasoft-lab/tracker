package main

import (
	"fmt"

	"github.com/khulnasoft-labs/tracker/signatures/helpers"
	"github.com/khulnasoft-labs/tracker/types/detect"
	"github.com/khulnasoft-labs/tracker/types/protocol"
	"github.com/khulnasoft-labs/tracker/types/trace"
)

type e2eSecurityInodeRename struct {
	cb detect.SignatureHandler
}

func (sig *e2eSecurityInodeRename) Init(ctx detect.SignatureContext) error {
	sig.cb = ctx.Callback
	return nil
}

func (sig *e2eSecurityInodeRename) GetMetadata() (detect.SignatureMetadata, error) {
	return detect.SignatureMetadata{
		ID:          "SECURITY_INODE_RENAME",
		EventName:   "SECURITY_INODE_RENAME",
		Version:     "0.1.0",
		Name:        "security_inode_rename Test",
		Description: "Instrumentation events E2E Tests: security_inode_rename",
		Tags:        []string{"e2e", "instrumentation"},
	}, nil
}

func (sig *e2eSecurityInodeRename) GetSelectedEvents() ([]detect.SignatureEventSelector, error) {
	return []detect.SignatureEventSelector{
		{Source: "tracker", Name: "security_inode_rename"},
	}, nil
}

func (sig *e2eSecurityInodeRename) OnEvent(event protocol.Event) error {
	eventObj, ok := event.Payload.(trace.Event)
	if !ok {
		return fmt.Errorf("failed to cast event's payload")
	}

	switch eventObj.EventName {
	case "security_inode_rename":
		oldPath, err := helpers.GetTrackerStringArgumentByName(eventObj, "old_path")
		if err != nil {
			return err
		}

		newPath, err := helpers.GetTrackerStringArgumentByName(eventObj, "new_path")
		if err != nil {
			return err
		}

		// check expected values from test for detection
		if oldPath != "/tmp/aaa.txt" || newPath != "/tmp/bb.txt" {
			return nil
		}

		m, _ := sig.GetMetadata()

		sig.cb(detect.Finding{
			SigMetadata: m,
			Event:       event,
			Data:        map[string]interface{}{},
		})
	}

	return nil
}

func (sig *e2eSecurityInodeRename) OnSignal(s detect.Signal) error {
	return nil
}

func (sig *e2eSecurityInodeRename) Close() {}
