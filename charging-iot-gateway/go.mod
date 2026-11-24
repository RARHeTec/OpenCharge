module github.com/RARHeTec/OpenCharge/charging-iot-gateway

go 1.22

require (
	github.com/RARHeTec/OpenCharge/charging-common v0.0.0
	github.com/RARHeTec/OpenCharge/charging-thing-model v0.0.0
)

replace github.com/RARHeTec/OpenCharge/charging-common => ../charging-common

replace github.com/RARHeTec/OpenCharge/charging-thing-model => ../charging-thing-model
