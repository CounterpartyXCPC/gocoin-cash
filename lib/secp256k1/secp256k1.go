package secp256k1

const WINDOW_A = 5
const WINDOW_G = 14
const FORCE_LOW_S = true // At the output of the Sign() function

var TheCurve struct {
	Order, HalfOrder Number
	G XY
	beta Field
	lambda, a1b2, b1, a2 Number
	p Number
}


func init_contants() {
	TheCurve.Order.SetBytes([]byte{
		0xFF,0xFF,0xFF,0xFF,0xFF,0xFF,0xFF,0xFF,0xFF,0xFF,0xFF,0xFF,0xFF,0xFF,0xFF,0xFE,
		0xBA,0xAE,0xDC,0xE6,0xAF,0x48,0xA0,0x3B,0xBF,0xD2,0x5E,0x8C,0xD0,0x36,0x41,0x41})

	TheCurve.HalfOrder.SetBytes([]byte{
		0X7F,0XFF,0XFF,0XFF,0XFF,0XFF,0XFF,0XFF,0XFF,0XFF,0XFF,0XFF,0XFF,0XFF,0XFF,0XFF,
		0X5D,0X57,0X6E,0X73,0X57,0XA4,0X50,0X1D,0XDF,0XE9,0X2F,0X46,0X68,0X1B,0X20,0XA0})

	TheCurve.p.SetBytes([]byte{
		0xFF,0xFF,0xFF,0xFF,0xFF,0xFF,0xFF,0xFF,0xFF,0xFF,0xFF,0xFF,0xFF,0xFF,0xFF,0xFF,
		0xFF,0xFF,0xFF,0xFF,0xFF,0xFF,0xFF,0xFF,0xFF,0xFF,0xFF,0xFE,0xFF,0xFF,0xFC,0x2F})

	TheCurve.G.X.SetB32([]byte{
		0x79,0xBE,0x66,0x7E,0xF9,0xDC,0xBB,0xAC,0x55,0xA0,0x62,0x95,0xCE,0x87,0x0B,0x07,
		0x02,0x9B,0xFC,0xDB,0x2D,0xCE,0x28,0xD9,0x59,0xF2,0x81,0x5B,0x16,0xF8,0x17,0x98})

	TheCurve.G.Y.SetB32([]byte{
		0x48,0x3A,0xDA,0x77,0x26,0xA3,0xC4,0x65,0x5D,0xA4,0xFB,0xFC,0x0E,0x11,0x08,0xA8,
		0xFD,0x17,0xB4,0x48,0xA6,0x85,0x54,0x19,0x9C,0x47,0xD0,0x8F,0xFB,0x10,0xD4,0xB8})

	TheCurve.lambda.SetBytes([]byte{
		0x53,0x63,0xad,0x4c,0xc0,0x5c,0x30,0xe0,0xa5,0x26,0x1c,0x02,0x88,0x12,0x64,0x5a,
		0x12,0x2e,0x22,0xea,0x20,0x81,0x66,0x78,0xdf,0x02,0x96,0x7c,0x1b,0x23,0xbd,0x72})

	TheCurve.beta.SetB32([]byte{
		0x7a,0xe9,0x6a,0x2b,0x65,0x7c,0x07,0x10,0x6e,0x64,0x47,0x9e,0xac,0x34,0x34,0xe9,
		0x9c,0xf0,0x49,0x75,0x12,0xf5,0x89,0x95,0xc1,0x39,0x6c,0x28,0x71,0x95,0x01,0xee})

	TheCurve.a1b2.SetBytes([]byte{
		0x30,0x86,0xd2,0x21,0xa7,0xd4,0x6b,0xcd,0xe8,0x6c,0x90,0xe4,0x92,0x84,0xeb,0x15})

	TheCurve.b1.SetBytes([]byte{
		0xe4,0x43,0x7e,0xd6,0x01,0x0e,0x88,0x28,0x6f,0x54,0x7f,0xa9,0x0a,0xbf,0xe4,0xc3})

	TheCurve.a2.SetBytes([]byte{
		0x01,0x14,0xca,0x50,0xf7,0xa8,0xe2,0xf3,0xf6,0x57,0xc1,0x10,0x8d,0x9d,0x44,0xcf,0xd8})
}


func init() {
	init_contants()
}