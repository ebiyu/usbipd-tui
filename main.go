package main

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

const (
	NotShared = iota
	Shared
	Attached
)

type UIState struct {
	AttachedItems *[]Device
	DetachedItems *[]Device
}

func main() {
	app := tview.NewApplication()

	detachedPane := tview.NewFlex()
	detachedPane.SetTitle("Detached (Attached to Host)").SetBorder(true)

	detachedTable := tview.NewTable()
	detachedPane.AddItem(detachedTable, 0, 1, true)

	attachedPane := tview.NewFlex()
	attachedPane.SetTitle("Attached to WSL").SetBorder(true)

	attachedTable := tview.NewTable()
	attachedPane.AddItem(attachedTable, 0, 1, true)

	flex := tview.NewFlex().
		AddItem(detachedPane, 0, 1, true).
		AddItem(attachedPane, 0, 1, true)

	flex.SetBorder(true)
	flex.SetTitle("USBIPD")

	uiState := UIState{
		AttachedItems: &[]Device{},
		DetachedItems: &[]Device{},
	}

	updateDeviceList := func() {
		attachedTable.Clear()
		detachedTable.Clear()

		items, err := getDevices()
		if err != nil {
			panic(err)
		}

		attachedItems := []Device{}
		for _, v := range items {
			if v.Status == Attached {
				attachedItems = append(attachedItems, v)
			}
		}
		detachedItems := []Device{}
		for _, v := range items {
			if v.Status != Attached {
				detachedItems = append(detachedItems, v)
			}
		}

		detachedTable.SetCell(0, 0, tview.NewTableCell("BusID").SetSelectable(false))
		detachedTable.SetCell(0, 1, tview.NewTableCell("DeviceID").SetSelectable(false))
		detachedTable.SetCell(0, 2, tview.NewTableCell("DeviceName").SetSelectable(false))
		for i, v := range detachedItems {
			detachedTable.SetCell(i+1, 0, tview.NewTableCell(v.BusID))
			detachedTable.SetCell(i+1, 1, tview.NewTableCell(v.DeviceID))
			detachedTable.SetCell(i+1, 2, tview.NewTableCell(v.DeviceName))
		}

		attachedTable.SetCell(0, 0, tview.NewTableCell("BusID").SetSelectable(false))
		attachedTable.SetCell(0, 1, tview.NewTableCell("DeviceID").SetSelectable(false))
		attachedTable.SetCell(0, 2, tview.NewTableCell("DeviceName").SetSelectable(false))

		for i, v := range attachedItems {
			attachedTable.SetCell(i+1, 0, tview.NewTableCell(v.BusID))
			attachedTable.SetCell(i+1, 1, tview.NewTableCell(v.DeviceID))
			attachedTable.SetCell(i+1, 2, tview.NewTableCell(v.DeviceName))
		}

		uiState.AttachedItems = &attachedItems
		uiState.DetachedItems = &detachedItems
	}

	updateDeviceList()

	detachedTable.SetSelectable(true, false)
	detachedTable.Select(1, 0)

	app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyRune && event.Rune() == 'r' {
			updateDeviceList()
			return nil
		}

		if event.Key() == tcell.KeyRune && event.Rune() == 'q' {
			app.Stop()
			return nil
		}

		return event
	})

	detachedTable.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyRight || (event.Key() == tcell.KeyRune && event.Rune() == 'l') {
			attachedTable.SetSelectable(true, false)
			detachedTable.SetSelectable(false, false)
			app.SetFocus(attachedTable)
			attachedTable.Select(1, 0)
			return nil
		}

		return event
	})

	attachedTable.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyLeft || (event.Key() == tcell.KeyRune && event.Rune() == 'h') {
			attachedTable.SetSelectable(false, false)
			detachedTable.SetSelectable(true, false)
			app.SetFocus(detachedTable)
			detachedTable.Select(1, 0)
			return nil
		}

		return event
	})

	detachedTable.SetSelectedFunc(func(row, column int) {
		device := (*uiState.DetachedItems)[row-1]
		if device.Status == NotShared {
			err := bindDevice(device.BusID)
			if err != nil {
				panic(err)
			}
		}

		err := attachDevice(device.BusID)
		if err != nil {
			panic(err)
		}

		updateDeviceList()
	})

	attachedTable.SetSelectedFunc(func(row, column int) {
		device := (*uiState.AttachedItems)[row-1]
		err := detachDevice(device.BusID)
		if err != nil {
			panic(err)
		}

		updateDeviceList()
	})

	app.SetFocus(detachedTable)

	if err := app.SetRoot(flex, true).Run(); err != nil {
		panic(err)
	}

}

type Device struct {
	BusID      string
	DeviceID   string
	DeviceName string
	Status     int
}

func getDevices() ([]Device, error) {
	cmd := exec.Command("usbipd.exe", "list")
	output, err := cmd.Output()
	if err != nil {
		return []Device{}, err
	}

	// make output list
	strOutput := string(output)
	strOutputList := strings.Split(strOutput, "\n")
	for i, v := range strOutputList {
		strOutputList[i] = strings.TrimSpace(v)
	}

	// Parse the output
	begnRow := -1
	endRow := -1

	for i, v := range strOutputList {
		if v == "Connected:" {
			begnRow = i + 2
		}
		if v == "Persisted:" {
			endRow = i
		}
	}
	if begnRow == -1 || endRow == -1 {
		return []Device{}, fmt.Errorf("Could not find the beginning or end of the device list")
	}

	items := []Device{}
	for _, v := range strOutputList[begnRow:endRow] {
		cols := strings.Fields(v)
		if len(cols) < 3 {
			continue
		}
		busid, device, remainder := cols[0], cols[1], cols[2:]
		status := NotShared
		if remainder[len(remainder)-1] == "Shared" {
			status = Shared
		} else if remainder[len(remainder)-1] == "Attached" {
			status = Attached
		}
		deviceName := strings.Join(remainder[:len(remainder)-1], " ")
		items = append(items, Device{
			BusID:      busid,
			DeviceID:   device,
			DeviceName: deviceName,
			Status:     status,
		})
	}

	return items, nil
}

func bindDevice(busid string) error {
	cmd := exec.Command("usbipd.exe", "bind", "--busid", busid)
	_, err := cmd.Output()
	if err != nil {
		return err
	}
	return nil
}

func attachDevice(busid string) error {
	cmd := exec.Command("usbipd.exe", "attach", "--wsl", "--busid", busid)
	_, err := cmd.Output()
	if err != nil {
		return err
	}
	return nil
}

func detachDevice(busid string) error {
	cmd := exec.Command("usbipd.exe", "detach", "--busid", busid)
	_, err := cmd.Output()
	if err != nil {
		return err
	}
	return nil
}
