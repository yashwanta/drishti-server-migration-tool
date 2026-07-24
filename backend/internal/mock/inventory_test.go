package mock

import "testing"

func TestConnectionsShape(t *testing.T) {
	p := New()
	conns := p.Connections()
	if len(conns) != 2 {
		t.Fatalf("expected 2 default connections, got %d", len(conns))
	}
	for _, c := range conns {
		if c.SecretRef == "" {
			t.Errorf("connection %s missing secret_ref", c.ID)
		}
		if c.Status == "" {
			t.Errorf("connection %s missing status", c.ID)
		}
	}
}

func TestVMwareInventory(t *testing.T) {
	p := New()
	inv, ok := p.Inventory("conn-vmware-lab")
	if !ok {
		t.Fatal("vmware inventory not found")
	}
	if len(inv.Datacenters) == 0 {
		t.Fatal("expected at least one datacenter")
	}
	var vmCount int
	for _, dc := range inv.Datacenters {
		for _, cl := range dc.Clusters {
			for _, h := range cl.Hosts {
				vmCount += len(h.VMs)
			}
		}
	}
	if vmCount == 0 {
		t.Error("expected VMs in vmware inventory")
	}
}

func TestProxmoxInventory(t *testing.T) {
	p := New()
	inv, ok := p.Inventory("conn-proxmox-lab")
	if !ok {
		t.Fatal("proxmox inventory not found")
	}
	if len(inv.Nodes) == 0 {
		t.Fatal("expected at least one node")
	}
	node := inv.Nodes[0]
	if !node.Online {
		t.Error("expected node online")
	}
	if len(node.Storage) == 0 {
		t.Error("expected storage on node")
	}
	if len(node.Bridges) == 0 {
		t.Error("expected bridges on node")
	}
}

func TestUnknownConnection(t *testing.T) {
	p := New()
	if _, ok := p.Inventory("does-not-exist"); ok {
		t.Error("expected not-found for unknown connection")
	}
}