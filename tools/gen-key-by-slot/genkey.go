package gen_key_by_slot

import (
	"fmt"
	"hash/crc32"
	"time"
)

// GenKeyBySlot 根据指定前缀和slot生成一个key名称.
// 用途:个别key比较大,可能占用超过15G,因此需要单独搭建一个大内存redis来专门存放;
// 又由于key名称是使用时生成的,因此需要能够根据指定slot生成key名称(slot固定则后端redis可固定).
func GenKeyBySlot(prefix string, slot int, maxSlot ...int) string {
	var defMaxSlot int = 1024
	if len(maxSlot) > 0 && maxSlot[0] > 0 {
		defMaxSlot = maxSlot[0]
	}

	tmpIndex := 0
	tmpPrefix := prefix + "_" + fmt.Sprintf("%d", time.Now().UnixNano()) + "_"
	for {
		tmpKey := fmt.Sprintf("%s%d", tmpPrefix, tmpIndex)
		if (int(crc32.ChecksumIEEE([]byte(tmpKey))) % defMaxSlot) != slot {
			tmpIndex++
			continue
		}
		return tmpKey
	}
	return ""
}
