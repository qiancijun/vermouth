package vermouth

import (
	"testing"
	"time"

	"github.com/hashicorp/raft"
	"github.com/stretchr/testify/assert"
)

func TestRaftCluster(t *testing.T) {
	leaderNotify := make(chan bool)
	leader, err := createRaftNode(raftRunArgs{
		tcpAddress:        "127.0.0.1:7181",
		serverName:        "leader",
		snapshotInterval:  3,
		snapshotThreshold: 1,
		heartbeatTimeout:  2,
		electionTimeout:   2,
		dataDir:           "../data",
		leader:            true,
	}, leaderNotify, NewStateContext())
	assert.NoError(t, err)
	follower1, err := createRaftNode(raftRunArgs{
		tcpAddress:        "127.0.0.1:7182",
		serverName:        "follower1",
		snapshotInterval:  3,
		snapshotThreshold: 1,
		heartbeatTimeout:  2,
		electionTimeout:   2,
		dataDir:           "../data",
		leader:            false,
	}, make(chan bool), NewStateContext())
	assert.NoError(t, err)
	follower2, err := createRaftNode(raftRunArgs{
		tcpAddress:        "127.0.0.1:7183",
		serverName:        "follower2",
		snapshotInterval:  3,
		snapshotThreshold: 1,
		heartbeatTimeout:  2,
		electionTimeout:   2,
		dataDir:           "../data",
		leader:            false,
	}, make(chan bool), NewStateContext())
	assert.NoError(t, err)
	<- leaderNotify
	addPeerFuture := leader.Raft.AddVoter(raft.ServerID("1"), follower1.Transport.LocalAddr(), 0, 0)
	assert.NoError(t, addPeerFuture.Error())
	addPeerFuture = leader.Raft.AddVoter(raft.ServerID("2"), follower2.Transport.LocalAddr(), 0, 0)
	assert.NoError(t, addPeerFuture.Error())
	
	// 发送一条创建新代理的 logEntry

	expect := []string{
		"192.168.3.10:8080",
		"192.168.3.11:8080",
		"192.168.3.12:8080",
	}

	err = leader.addHttpProxy(&addHttpProxyBody{9000, "hash-prefixer"})
	assert.NoError(t, err)
	time.Sleep(2 * time.Second)
	err = leader.addHttpProxyPrefix(&addHttpProxyPrefixBody{9000, false, "/api", "round-robin", expect})
	time.Sleep(2 * time.Second)
	assert.NoError(t, err)

	actual := leader.ctx.HttpReverseProxyList[9000].GetHosts("/api")
	assert.Equal(t, expect, actual)

	actual = follower1.ctx.HttpReverseProxyList[9000].GetHosts("/api")
	assert.Equal(t, expect, actual)

	actual = follower2.ctx.HttpReverseProxyList[9000].GetHosts("/api")
	assert.Equal(t, expect, actual)
}
