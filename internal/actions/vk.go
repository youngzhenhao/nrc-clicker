package actions

// VK → AT Set 2 scancode mapping, lifted verbatim from ActionScript.py
// VK_TO_SCANCODE so existing JSON scripts keep working.
//
// The bool indicates whether the scancode is an extended (E0) key — arrow
// keys, numpad enter, etc. require the KEY_E0 bit set in the KeyStroke
// state field.
type vkSpec struct {
	Scancode uint16
	Extended bool
}

// VKToScancode maps Windows virtual key codes to their AT Set 2 hardware
// scancodes. Returns (spec, true) on a hit, (zero value, false) when the
// VK has no known mapping (caller should fall back to keybd_event).
var VKToScancode = map[uint16]vkSpec{
	0x08: {0x0E, false}, // VK_BACK
	0x09: {0x0F, false}, // VK_TAB
	0x0D: {0x1C, false}, // VK_RETURN
	0x10: {0x2A, false}, // VK_SHIFT
	0x11: {0x1D, false}, // VK_CONTROL
	0x12: {0x38, false}, // VK_MENU (alt)
	0x13: {0x45, false}, // VK_PAUSE
	0x14: {0x3A, false}, // VK_CAPITAL
	0x1B: {0x01, false}, // VK_ESCAPE
	0x20: {0x39, false}, // VK_SPACE
	0x21: {0x49, true},  // VK_PRIOR (Page Up)
	0x22: {0x51, true},  // VK_NEXT (Page Down)
	0x23: {0x4F, true},  // VK_END
	0x24: {0x47, true},  // VK_HOME
	0x25: {0x4B, true},  // VK_LEFT
	0x26: {0x48, true},  // VK_UP
	0x27: {0x4D, true},  // VK_RIGHT
	0x28: {0x50, true},  // VK_DOWN
	0x2D: {0x53, true},  // VK_INSERT
	0x2E: {0x53, true},  // VK_DELETE
	0x30: {0x0B, false}, // '0'
	0x31: {0x02, false}, // '1'
	0x32: {0x03, false}, // '2'
	0x33: {0x04, false}, // '3'
	0x34: {0x05, false}, // '4'
	0x35: {0x06, false}, // '5'
	0x36: {0x07, false}, // '6'
	0x37: {0x08, false}, // '7'
	0x38: {0x09, false}, // '8'
	0x39: {0x0A, false}, // '9'
	0x41: {0x1E, false}, // 'A'
	0x42: {0x30, false}, // 'B'
	0x43: {0x2E, false}, // 'C'
	0x44: {0x20, false}, // 'D'
	0x45: {0x12, false}, // 'E'
	0x46: {0x21, false}, // 'F'
	0x47: {0x22, false}, // 'G'
	0x48: {0x23, false}, // 'H'
	0x49: {0x17, false}, // 'I'
	0x4A: {0x24, false}, // 'J'
	0x4B: {0x25, false}, // 'K'
	0x4C: {0x26, false}, // 'L'
	0x4D: {0x32, false}, // 'M'
	0x4E: {0x31, false}, // 'N'
	0x4F: {0x18, false}, // 'O'
	0x50: {0x19, false}, // 'P'
	0x51: {0x10, false}, // 'Q'
	0x52: {0x13, false}, // 'R'
	0x53: {0x1F, false}, // 'S'
	0x54: {0x14, false}, // 'T'
	0x55: {0x16, false}, // 'U'
	0x56: {0x2F, false}, // 'V'
	0x57: {0x11, false}, // 'W'
	0x58: {0x2D, false}, // 'X'
	0x59: {0x15, false}, // 'Y'
	0x5A: {0x2C, false}, // 'Z'
	0x60: {0x52, false}, // Numpad 0
	0x61: {0x4F, false}, // Numpad 1
	0x62: {0x50, false}, // Numpad 2
	0x63: {0x51, false}, // Numpad 3
	0x64: {0x4B, false}, // Numpad 4
	0x65: {0x4C, false}, // Numpad 5
	0x66: {0x4D, false}, // Numpad 6
	0x67: {0x47, false}, // Numpad 7
	0x68: {0x48, false}, // Numpad 8
	0x69: {0x49, false}, // Numpad 9
	0x6F: {0x35, true},  // VK_DIVIDE
	0x70: {0x3B, false}, // F1
	0x71: {0x3C, false}, // F2
	0x72: {0x3D, false}, // F3
	0x73: {0x3E, false}, // F4
	0x74: {0x3F, false}, // F5
	0x75: {0x40, false}, // F6
	0x76: {0x41, false}, // F7
	0x77: {0x42, false}, // F8
	0x78: {0x43, false}, // F9
	0x79: {0x44, false}, // F10
	0x7A: {0x57, false}, // F11
	0x7B: {0x58, false}, // F12
	0xA0: {0x2A, false}, // VK_LSHIFT
	0xA1: {0x36, false}, // VK_RSHIFT
	0xA2: {0x1D, false}, // VK_LCONTROL
	0xA3: {0x1D, true},  // VK_RCONTROL
	0xA4: {0x38, false}, // VK_LMENU
	0xA5: {0x38, true},  // VK_RMENU
}

// LookupScancode returns the AT Set 2 scancode and extended flag for a VK
// code, or (0, false) when the VK is unknown. The caller should fall back
// to user32.keybd_event in that case.
func LookupScancode(vk uint16) (scancode uint16, extended bool, ok bool) {
	s, ok := VKToScancode[vk]
	if !ok {
		return 0, false, false
	}
	return s.Scancode, s.Extended, true
}
