package bhyve

import (
	"context"
	"fmt"
	"log"
	"os/exec"
	"sync"

	"github.com/hashicorp/packer-plugin-sdk/multistep"
	"github.com/hashicorp/packer-plugin-sdk/packer"
)

type stepBhyve struct {
	name    string
	vmEndCh <-chan int
	lock    sync.Mutex
}

func (step *stepBhyve) Run(ctx context.Context, state multistep.StateBag) multistep.StepAction {
	config := state.Get("config").(*Config)
	ui := state.Get("ui").(packer.Ui)

	// Use the same slots as pci_slot_t in
	// illumos-joyent usr/src/lib/brand/bhyve/zone/boot.c
	const (
		SlotHostBridge int = 0
		SlotCDROM          = 3
		SlotBootDisk       = 4
		SlotNIC            = 6
		SlotFBuf           = 30
		SlotLPC            = 31
	)

	args := []string{
		"-D",
		"-H",
		"-c", "1",
		"-l", "bootrom,/usr/share/bhyve/uefi-rom.bin",
		"-m", "1024",
		"-s", fmt.Sprintf("%d,hostbridge,model=i440fx", SlotHostBridge),
		"-s", fmt.Sprintf("%d,ahci-cd,%s", SlotCDROM,
			state.Get("iso_path").(string)),
		"-s", fmt.Sprintf("%d,virtio-blk,/dev/zvol/rdsk/%s/packer0",
			SlotBootDisk, config.ZPool),
		"-s", fmt.Sprintf("%d,virtio-net-viona,vnic=packer0", SlotNIC),
		"-s", fmt.Sprintf("%d:0,fbuf,vga=off,rfb=%s:%d,password=%s",
			SlotFBuf, config.VNCBindAddress,
			state.Get("vnc_port").(int),
			state.Get("vnc_password").(string)),
		"-s", fmt.Sprintf("%d:1,xhci,tablet", SlotFBuf),
		"-s", fmt.Sprintf("%d,lpc", SlotLPC),
		step.name,
	}

	step.lock.Lock()
	defer step.lock.Unlock()

	ui.Say(fmt.Sprintf("Starting bhyve VM %s", step.name))

	cmd := exec.Command("/usr/sbin/bhyve", args...)
	if err := cmd.Start(); err != nil {
		err = fmt.Errorf("Error starting VM: %s", err)
		return multistep.ActionHalt
	}

	// bhyve exits when a VM reboots which is a bit annoying in this
	// context.  We need to check for this and restart it on success so
	// that the post-install provisioning step can run.  Once complete
	// the VM is powered off which is a non-zero exit status.
	endCh := make(chan int, 1)
	go func() {
		var rc int = 0
		if err := cmd.Wait(); err == nil {
			ui.Say(fmt.Sprintf("Restarting bhyve VM %s after reboot", step.name))
			cmd := exec.Command("/usr/sbin/bhyve", args...)
			if err := cmd.Start(); err != nil {
				// XXX: Report this as failing to packer
				ui.Say(fmt.Sprintf("Error restarting VM: %s", err))
			}
		} else {
			if status, ok := err.(*exec.ExitError); ok {
				rc = status.ExitCode()
			}
		}

		endCh <- rc
		step.lock.Lock()
		defer step.lock.Unlock()
		step.vmEndCh = nil
	}()

	step.vmEndCh = endCh

	return multistep.ActionContinue
}

func (step *stepBhyve) Cleanup(state multistep.StateBag) {
	ui := state.Get("ui").(packer.Ui)

	vmarg := fmt.Sprintf("--vm=%s", step.name)

	args := []string{
		vmarg,
		"--destroy",
	}

	ui.Say(fmt.Sprintf("Stopping bhyve VM %s", step.name))

	cmd := exec.Command("/usr/sbin/bhyvectl", args...)
	if err := cmd.Run(); err != nil {
		log.Printf("Error stopping VM: %s", err)
	}
}
