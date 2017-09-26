package gen_key_by_slot

import (
	"testing"
)

func TestGenKeyBySlot(t *testing.T) {
	key := GenKeyBySlot("db_msg_id", 180)
	t.Logf("slot:%d,key:%s", 180, key)
}
