package device

import "github.com/RARHeTec/OpenCharge/charging-common/lock"

func lockKey(deviceID string, gunNo int) string {
	return lock.Key(deviceID, gunNo)
}
