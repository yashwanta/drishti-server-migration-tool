package vmware

import (
	"context"
	"testing"

	"github.com/vmware/govmomi/simulator"
)

func TestInventoryFromStandaloneESXi(t *testing.T) {
	model := simulator.ESX()
	model.Machine = 2
	defer model.Remove()
	if err := model.Create(); err != nil {
		t.Fatalf("create simulator: %v", err)
	}
	server := model.Service.NewServer()
	defer server.Close()

	username := server.URL.User.Username()
	password, _ := server.URL.User.Password()
	client := New(server.URL.String(), username, password, true)
	inv, err := client.Inventory(context.Background(), "conn-esxi-test")
	if err != nil {
		t.Fatalf("Inventory: %v", err)
	}
	if inv.ConnectionID != "conn-esxi-test" {
		t.Fatalf("connection id = %q", inv.ConnectionID)
	}
	vmCount := 0
	for _, dc := range inv.Datacenters {
		for _, cluster := range dc.Clusters {
			for _, host := range cluster.Hosts {
				vmCount += len(host.VMs)
			}
		}
	}
	if vmCount != 2 {
		t.Fatalf("VM count = %d, want 2", vmCount)
	}
}
