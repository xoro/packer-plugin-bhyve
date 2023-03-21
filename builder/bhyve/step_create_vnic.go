package bhyve

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os/exec"
	"strings"

	"github.com/hashicorp/packer-plugin-sdk/multistep"
	"github.com/hashicorp/packer-plugin-sdk/packer"
)

type stepCreateVNIC struct {
	name string
}

func (step *stepCreateVNIC) Run(ctx context.Context, state multistep.StateBag) multistep.StepAction {
	config := state.Get("config").(*Config)
	ui := state.Get("ui").(packer.Ui)

	args := []string{
		"create-vnic",
		"-l", config.HostNIC,
		"packer0",
	}

	ui.Say(fmt.Sprintf("Creating VNIC packer0 on link %s", config.HostNIC))

	cmd := exec.Command("/usr/sbin/dladm", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		err := fmt.Errorf("Error creating VNIC: %s", strings.TrimSpace(stderr.String()))
		state.Put("error", err)
		ui.Error(err.Error())
		return multistep.ActionHalt
	}

	return multistep.ActionContinue
}

func (step *stepCreateVNIC) Cleanup(state multistep.StateBag) {
	config := state.Get("config").(*Config)
	ui := state.Get("ui").(packer.Ui)

	args := []string{
		"delete-vnic",
		"packer0",
	}

	ui.Say(fmt.Sprintf("Deleting VNIC packer0 from link %s", config.HostNIC))

	cmd := exec.Command("/usr/sbin/dladm", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		log.Printf("Error deleting VNIC: %s", strings.TrimSpace(stderr.String()))
	}
}
