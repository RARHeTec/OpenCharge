package protocol

// Frame type codes from section 4 of the YunKuaiChong V1.6 spec.
const (
	TypeLogin            byte = 0x01 // 充电桩登录认证 (up)
	TypeLoginAck         byte = 0x02 // 登录认证应答 (down)
	TypeHeartbeat        byte = 0x03 // 心跳包 (up)
	TypeHeartbeatAck     byte = 0x04 // 心跳应答 (down)
	TypeTariffVerify     byte = 0x05 // 计费模型验证请求 (up)
	TypeTariffVerifyAck  byte = 0x06 // 计费模型验证应答 (down)
	TypeTariffRequest    byte = 0x09 // 计费模型请求 (up)
	TypeTariffRequestAck byte = 0x0A // 计费模型请求应答 (down)

	TypeReadRealtime     byte = 0x12 // 读取实时监测数据 (down)
	TypeRealtime         byte = 0x13 // 实时监测数据 (up)
	TypeHandshake        byte = 0x15 // 充电握手 (up)
	TypeParamConfig      byte = 0x17 // 参数配置 (up)
	TypeChargeEnd        byte = 0x19 // 充电结束 (up)
	TypeError            byte = 0x1B // 错误报文 (up)

	TypeStartRequest     byte = 0x31 // 桩主动申请启动充电 (up)
	TypeStartConfirm     byte = 0x32 // 确认启动充电 (down)
	TypeRemoteStartAck   byte = 0x33 // 远程启机命令回复 (up)
	TypeRemoteStart      byte = 0x34 // 远程控制启机 (down)
	TypeRemoteStopAck    byte = 0x35 // 远程停机命令回复 (up)
	TypeRemoteStop       byte = 0x36 // 远程停机 (down)
	TypeTransaction      byte = 0x3B // 交易记录 (up)
	TypeTransactionAck   byte = 0x40 // 交易记录确认 (down)
	// Tariff model downlink (section 9.5/9.6 of the V1.6 spec).
	TypeTariffSet     byte = 0x58 // 计费模型设置 (down), 90-byte body
	TypeTariffSetAck  byte = 0x57 // 计费模型应答 (up), 8-byte body

	TypeBalanceUpdateAck byte = 0x41 // 余额更新应答 (up)
	TypeBalanceUpdate    byte = 0x42 // 远程账户余额更新 (down)
)
