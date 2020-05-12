// Commands from https://redis.io/commands#cluster

package miniredis

import (
	"fmt"
	"github.com/alicebob/miniredis/v2/server"
	"strings"
)

// commandsCluster handles some cluster operations.
func commandsCluster(m *Miniredis) {
	_ = m.srv.Register("CLUSTER", m.cmdCluster)
}

func (m *Miniredis) cmdCluster(c *server.Peer, cmd string, args []string) {
	if len(args) == 1 && strings.ToUpper(args[0]) == "SLOTS" {
		m.cmdClusterSlots(c, cmd, args)
	} else if len(args) == 2 && strings.ToUpper(args[0]) == "KEYSLOT" {
		m.cmdClusterKeySlot(c, cmd, args)
	} else if len(args) == 1 && strings.ToUpper(args[0]) == "NODES" {
		m.cmdClusterNodes(c, cmd, args)
	} else {
		j := strings.Join(args, " ")
		err := fmt.Sprintf("ERR 'CLUSTER %s' not supported", j)
		setDirty(c)
		c.WriteError(err)
	}
}

// CLUSTER SLOTS
func (m *Miniredis) cmdClusterSlots(c *server.Peer, cmd string, args []string) {
	withTx(m, c, func(c *server.Peer, ctx *connCtx) {
		c.WriteLen(1)
		c.WriteLen(3)
		c.WriteInt(0)
		c.WriteInt(16383)
		c.WriteLen(3)
		c.WriteBulk(m.srv.Addr().IP.String())
		c.WriteInt(m.srv.Addr().Port)
		c.WriteBulk("09dbe9720cda62f7865eabc5fd8857c5d2678366")
	})
}

//CLUSTER KEYSLOT
func (m *Miniredis) cmdClusterKeySlot(c *server.Peer, cmd string, args []string) {
	withTx(m, c, func(c *server.Peer, ctx *connCtx) {
		c.WriteInt(163)
	})
}

//CLUSTER NODES
func (m *Miniredis) cmdClusterNodes(c *server.Peer, cmd string, args []string) {
	withTx(m, c, func(c *server.Peer, ctx *connCtx) {
		c.WriteBulk("e7d1eecce10fd6bb5eb35b9f99a514335d9ba9ca 127.0.0.1:7000@7000 myself,master - 0 0 1 connected 0-16383")
	})
}

