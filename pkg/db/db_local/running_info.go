package db_local

import (
	"bytes"
	"encoding/json"
	"log"
	"os"
	"os/exec"
	"sort"

	filehelpers "github.com/turbot/go-kit/files"
	"github.com/turbot/go-kit/helpers"
	"github.com/turbot/steampipe/pkg/constants"
	"github.com/turbot/steampipe/pkg/filepaths"
	"github.com/turbot/steampipe/pkg/utils"
)

const RunningDBStructVersion = 20220411

// RunningDBInstanceInfo contains data about the running process and it's credentials
type RunningDBInstanceInfo struct {
	Pid             int               `json:"pid"`
	ListenAddresses []string          `json:"listen"`
	Port            int               `json:"port"`
	Invoker         constants.Invoker `json:"invoker"`
	Password        string            `json:"password"`
	User            string            `json:"user"`
	Database        string            `json:"database"`
	StructVersion   int64             `json:"struct_version"`
}

func newRunningDBInstanceInfo(cmd *exec.Cmd, listenAddresses []string, port int, databaseName string, password string, invoker constants.Invoker) *RunningDBInstanceInfo {
	listenAddresses = getListenAddresses(listenAddresses)

	dbState := &RunningDBInstanceInfo{
		Pid:             cmd.Process.Pid,
		ListenAddresses: listenAddresses,
		Port:            port,
		User:            constants.DatabaseUser,
		Password:        password,
		Database:        databaseName,
		Invoker:         invoker,
		StructVersion:   RunningDBStructVersion,
	}

	return dbState
}

func getListenAddresses(listenAddresses []string) []string {
	addresses := []string{}

	if helpers.StringSliceContains(listenAddresses, "localhost") {
		loopAddrs, err := utils.LocalLoopbackAddresses()
		if err != nil {
			return nil
		}
		addresses = loopAddrs
	}

	if helpers.StringSliceContains(listenAddresses, "*") {
		// remove the * wildcard, we want to replace that with the actual addresses
		listenAddresses = helpers.RemoveFromStringSlice(listenAddresses, "*")
		loopAddrs, err := utils.LocalLoopbackAddresses()
		if err != nil {
			return nil
		}
		publicAddrs, err := utils.LocalPublicAddresses()
		if err != nil {
			return nil
		}
		addresses = append(loopAddrs, publicAddrs...)
	}

	// now add back the listenAddresses to address arguments where the interface addresses were sent
	addresses = append(addresses, listenAddresses...)
	addresses = helpers.StringSliceDistinct(addresses)

	// sort locals to the top
	sort.SliceStable(addresses, func(i, j int) bool {
		locals := []string{
			"127.0.0.1",
			"::1",
			"localhost",
		}
		return !helpers.StringSliceContains(locals, addresses[j])
	})

	return addresses
}

func (r *RunningDBInstanceInfo) Save() error {
	// set struct version
	r.StructVersion = RunningDBStructVersion

	content, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepaths.RunningInfoFilePath(), content, 0644)
}

func (r *RunningDBInstanceInfo) String() string {
	writeBuffer := bytes.NewBufferString("")
	jsonEncoder := json.NewEncoder(writeBuffer)

	// redact the password from the string, so that it doesn't get printed
	// this should not affect the state file, since we use a json.Marshal there
	p := r.Password
	r.Password = "XXXX-XXXX-XXXX"

	jsonEncoder.SetIndent("", "")
	err := jsonEncoder.Encode(r)
	if err != nil {
		log.Printf("[TRACE] Encode failed: %v\n", err)
	}
	r.Password = p
	return writeBuffer.String()
}

func loadRunningInstanceInfo() (*RunningDBInstanceInfo, error) {
	utils.LogTime("db.loadRunningInstanceInfo start")
	defer utils.LogTime("db.loadRunningInstanceInfo end")

	if !filehelpers.FileExists(filepaths.RunningInfoFilePath()) {
		return nil, nil
	}

	fileContent, err := os.ReadFile(filepaths.RunningInfoFilePath())
	if err != nil {
		return nil, err
	}
	var info = new(RunningDBInstanceInfo)
	err = json.Unmarshal(fileContent, info)
	if err != nil {
		log.Printf("[TRACE] failed to unmarshal database state file %s: %s\n", filepaths.RunningInfoFilePath(), err.Error())
		return nil, nil
	}
	return info, nil
}

func removeRunningInstanceInfo() error {
	return os.Remove(filepaths.RunningInfoFilePath())
}
