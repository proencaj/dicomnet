package dicom

import (
	"encoding/binary"
	"testing"
)

func TestTag_String(t *testing.T) {
	tests := []struct {
		name     string
		tag      Tag
		expected string
	}{
		{"Patient Name", Tag{0x0010, 0x0010}, "(0010,0010)"},
		{"Study Instance UID", Tag{0x0020, 0x000D}, "(0020,000d)"},
		{"Series Instance UID", Tag{0x0020, 0x000E}, "(0020,000e)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.tag.String()
			if result != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestNewDataset(t *testing.T) {
	ds := NewDataset()
	if ds == nil {
		t.Fatal("NewDataset returned nil")
	}
	if ds.Elements == nil {
		t.Error("Elements map is nil")
	}
	if len(ds.Elements) != 0 {
		t.Errorf("Expected empty dataset, got %d elements", len(ds.Elements))
	}
}

func TestDataset_AddElement(t *testing.T) {
	ds := NewDataset()

	tag := Tag{0x0010, 0x0010}
	vr := VR_PN
	value := "DOE^JOHN"

	ds.AddElement(tag, vr, value)

	element, exists := ds.GetElement(tag)
	if !exists {
		t.Fatal("Element not found after adding")
	}

	if element.Tag != tag {
		t.Errorf("Tag mismatch: expected %v, got %v", tag, element.Tag)
	}
	if element.VR != vr {
		t.Errorf("VR mismatch: expected %s, got %s", vr, element.VR)
	}
	if element.Value != value {
		t.Errorf("Value mismatch: expected %s, got %v", value, element.Value)
	}
}

func TestDataset_GetElement(t *testing.T) {
	ds := NewDataset()

	existingTag := Tag{0x0010, 0x0020}
	ds.AddElement(existingTag, VR_LO, "12345")

	// Test existing element
	element, exists := ds.GetElement(existingTag)
	if !exists {
		t.Error("Expected to find existing element")
	}
	if element == nil {
		t.Fatal("Element is nil")
	}

	// Test non-existing element
	nonExistingTag := Tag{0xFFFF, 0xFFFF}
	element, exists = ds.GetElement(nonExistingTag)
	if exists {
		t.Error("Expected not to find non-existing element")
	}
	if element != nil {
		t.Error("Element should be nil for non-existing tag")
	}
}

func TestDataset_GetString(t *testing.T) {
	ds := NewDataset()

	tests := []struct {
		name     string
		tag      Tag
		value    interface{}
		expected string
	}{
		{"String value", Tag{0x0010, 0x0010}, "DOE^JOHN", "DOE^JOHN"},
		{"String with spaces", Tag{0x0010, 0x0020}, "  12345  ", "12345"},
		{"Non-string value", Tag{0x0020, 0x0011}, 123, ""},
		{"Non-existing tag", Tag{0xFFFF, 0xFFFF}, nil, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.value != nil {
				ds.AddElement(tt.tag, VR_LO, tt.value)
			}
			result := ds.GetString(tt.tag)
			if result != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestDataset_GetStrings(t *testing.T) {
	ds := NewDataset()

	tests := []struct {
		name     string
		tag      Tag
		value    interface{}
		expected []string
	}{
		{
			name:     "Single value",
			tag:      Tag{0x0008, 0x0060},
			value:    "CT",
			expected: []string{"CT"},
		},
		{
			name:     "Multiple values with backslash",
			tag:      Tag{0x0008, 0x0008},
			value:    "ORIGINAL\\PRIMARY\\AXIAL",
			expected: []string{"ORIGINAL", "PRIMARY", "AXIAL"},
		},
		{
			name:     "String slice",
			tag:      Tag{0x0008, 0x0018},
			value:    []string{"value1", "value2"},
			expected: []string{"value1", "value2"},
		},
		{
			name:     "Non-string value",
			tag:      Tag{0x0020, 0x0013},
			value:    123,
			expected: nil,
		},
		{
			name:     "Non-existing tag",
			tag:      Tag{0xFFFF, 0xFFFF},
			value:    nil,
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.value != nil {
				ds.AddElement(tt.tag, VR_CS, tt.value)
			}
			result := ds.GetStrings(tt.tag)
			if len(result) != len(tt.expected) {
				t.Errorf("Expected %d strings, got %d", len(tt.expected), len(result))
				return
			}
			for i := range result {
				if result[i] != tt.expected[i] {
					t.Errorf("String[%d]: expected %q, got %q", i, tt.expected[i], result[i])
				}
			}
		})
	}
}

func TestParseDataset(t *testing.T) {
	tests := []struct {
		name        string
		data        []byte
		expectedLen int
		checks      func(t *testing.T, ds *Dataset)
	}{
		{
			name:        "Empty dataset",
			data:        []byte{},
			expectedLen: 0,
		},
		{
			name: "Single element",
			data: func() []byte {
				// Explicit VR: Tag (4) + VR (2) + Length (2) + Value
				// Tag 0010,0010 (Patient Name)
				data := make([]byte, 8)
				binary.LittleEndian.PutUint16(data[0:2], 0x0010) // Group
				binary.LittleEndian.PutUint16(data[2:4], 0x0010) // Element
				data[4] = 'P'                                    // VR
				data[5] = 'N'
				binary.LittleEndian.PutUint16(data[6:8], 8) // Length (2 bytes for short VR)
				data = append(data, []byte("DOE^JOHN")...)
				return data
			}(),
			expectedLen: 1,
			checks: func(t *testing.T, ds *Dataset) {
				value := ds.GetString(Tag{0x0010, 0x0010})
				if value != "DOE^JOHN" {
					t.Errorf("Expected DOE^JOHN, got %s", value)
				}
			},
		},
		{
			name: "Multiple elements",
			data: func() []byte {
				var data []byte

				// Explicit VR: Tag (4) + VR (2) + Length (2) + Value
				// Tag 0010,0010 (Patient Name)
				tag1 := make([]byte, 8)
				binary.LittleEndian.PutUint16(tag1[0:2], 0x0010) // Group
				binary.LittleEndian.PutUint16(tag1[2:4], 0x0010) // Element
				tag1[4] = 'P'                                    // VR
				tag1[5] = 'N'
				name := []byte("DOE^JOHN")
				binary.LittleEndian.PutUint16(tag1[6:8], uint16(len(name))) // Length (2 bytes)
				data = append(data, tag1...)
				data = append(data, name...)

				// Tag 0010,0020 (Patient ID)
				tag2 := make([]byte, 8)
				binary.LittleEndian.PutUint16(tag2[0:2], 0x0010) // Group
				binary.LittleEndian.PutUint16(tag2[2:4], 0x0020) // Element
				tag2[4] = 'L'                                    // VR
				tag2[5] = 'O'
				id := []byte("12345")
				binary.LittleEndian.PutUint16(tag2[6:8], uint16(len(id))) // Length (2 bytes)
				data = append(data, tag2...)
				data = append(data, id...)

				return data
			}(),
			expectedLen: 2,
			checks: func(t *testing.T, ds *Dataset) {
				name := ds.GetString(Tag{0x0010, 0x0010})
				if name != "DOE^JOHN" {
					t.Errorf("Expected DOE^JOHN, got %s", name)
				}
				id := ds.GetString(Tag{0x0010, 0x0020})
				if id != "12345" {
					t.Errorf("Expected 12345, got %s", id)
				}
			},
		},
		{
			name: "Element with odd length (requires padding)",
			data: func() []byte {
				// Explicit VR: Tag (4) + VR (2) + Length (2) + Value
				// Tag 0010,0010 (Patient Name) with 7 bytes (odd)
				data := make([]byte, 8)
				binary.LittleEndian.PutUint16(data[0:2], 0x0010) // Group
				binary.LittleEndian.PutUint16(data[2:4], 0x0010) // Element
				data[4] = 'P'                                    // VR
				data[5] = 'N'
				binary.LittleEndian.PutUint16(data[6:8], 7) // Odd length (2 bytes)
				data = append(data, []byte("JOHNSON")...)
				data = append(data, 0x20) // Padding byte
				return data
			}(),
			expectedLen: 1,
			checks: func(t *testing.T, ds *Dataset) {
				value := ds.GetString(Tag{0x0010, 0x0010})
				if value != "JOHNSON" {
					t.Errorf("Expected JOHNSON, got %s", value)
				}
			},
		},
		{
			// Regression test: GE Secondary Capture instances carry an
			// undefined-length Procedure Code Sequence (0008,1032) before
			// group 0020. Undefined length used to make the parser break
			// out of the loop entirely, silently dropping StudyInstanceUID.
			name: "Undefined-length SQ before StudyInstanceUID",
			data: func() []byte {
				var data []byte

				// (0008,1032) Procedure Code Sequence, VR=SQ, undefined length
				sq := make([]byte, 12)
				binary.LittleEndian.PutUint16(sq[0:2], 0x0008)
				binary.LittleEndian.PutUint16(sq[2:4], 0x1032)
				sq[4] = 'S'
				sq[5] = 'Q'
				// sq[6:8] reserved
				binary.LittleEndian.PutUint32(sq[8:12], undefinedLength)
				data = append(data, sq...)

				// One Item, defined length, arbitrary payload
				itemPayload := []byte("PLACEHOLDER!") // 12 bytes, even
				item := make([]byte, 8)
				binary.LittleEndian.PutUint16(item[0:2], 0xFFFE)
				binary.LittleEndian.PutUint16(item[2:4], 0xE000)
				binary.LittleEndian.PutUint32(item[4:8], uint32(len(itemPayload)))
				data = append(data, item...)
				data = append(data, itemPayload...)

				// Sequence Delimitation Item, length 0
				seqDelim := make([]byte, 8)
				binary.LittleEndian.PutUint16(seqDelim[0:2], 0xFFFE)
				binary.LittleEndian.PutUint16(seqDelim[2:4], 0xE0DD)
				binary.LittleEndian.PutUint32(seqDelim[4:8], 0)
				data = append(data, seqDelim...)

				// (0020,000D) Study Instance UID, VR=UI, short form
				uid := []byte("1.2.840.113619.2.1.0")
				if len(uid)%2 == 1 {
					uid = append(uid, 0x00)
				}
				study := make([]byte, 8)
				binary.LittleEndian.PutUint16(study[0:2], 0x0020)
				binary.LittleEndian.PutUint16(study[2:4], 0x000D)
				study[4] = 'U'
				study[5] = 'I'
				binary.LittleEndian.PutUint16(study[6:8], uint16(len(uid)))
				data = append(data, study...)
				data = append(data, uid...)

				return data
			}(),
			expectedLen: 2,
			checks: func(t *testing.T, ds *Dataset) {
				uid := ds.GetString(Tag{0x0020, 0x000D})
				if uid != "1.2.840.113619.2.1.0" {
					t.Errorf("Expected StudyInstanceUID 1.2.840.113619.2.1.0, got %q", uid)
				}
			},
		},
		{
			// Nested undefined length: the item inside the sequence also has
			// undefined length, ended by an Item Delimitation Item, exercising
			// skipItemStream's recursive branch.
			name: "Nested undefined-length item inside undefined-length SQ",
			data: func() []byte {
				var data []byte

				sq := make([]byte, 12)
				binary.LittleEndian.PutUint16(sq[0:2], 0x0008)
				binary.LittleEndian.PutUint16(sq[2:4], 0x1032)
				sq[4] = 'S'
				sq[5] = 'Q'
				binary.LittleEndian.PutUint32(sq[8:12], undefinedLength)
				data = append(data, sq...)

				// Item with undefined length
				item := make([]byte, 8)
				binary.LittleEndian.PutUint16(item[0:2], 0xFFFE)
				binary.LittleEndian.PutUint16(item[2:4], 0xE000)
				binary.LittleEndian.PutUint32(item[4:8], undefinedLength)
				data = append(data, item...)

				// A short element inside the item (0008,0100) Code Value, VR=SH
				code := []byte("AB")
				inner := make([]byte, 8)
				binary.LittleEndian.PutUint16(inner[0:2], 0x0008)
				binary.LittleEndian.PutUint16(inner[2:4], 0x0100)
				inner[4] = 'S'
				inner[5] = 'H'
				binary.LittleEndian.PutUint16(inner[6:8], uint16(len(code)))
				data = append(data, inner...)
				data = append(data, code...)

				// Item Delimitation Item, length 0
				itemDelim := make([]byte, 8)
				binary.LittleEndian.PutUint16(itemDelim[0:2], 0xFFFE)
				binary.LittleEndian.PutUint16(itemDelim[2:4], 0xE00D)
				binary.LittleEndian.PutUint32(itemDelim[4:8], 0)
				data = append(data, itemDelim...)

				// Sequence Delimitation Item, length 0
				seqDelim := make([]byte, 8)
				binary.LittleEndian.PutUint16(seqDelim[0:2], 0xFFFE)
				binary.LittleEndian.PutUint16(seqDelim[2:4], 0xE0DD)
				binary.LittleEndian.PutUint32(seqDelim[4:8], 0)
				data = append(data, seqDelim...)

				// (0020,000D) Study Instance UID
				uid := []byte("1.2.3.4.5.6")
				if len(uid)%2 == 1 {
					uid = append(uid, 0x00)
				}
				study := make([]byte, 8)
				binary.LittleEndian.PutUint16(study[0:2], 0x0020)
				binary.LittleEndian.PutUint16(study[2:4], 0x000D)
				study[4] = 'U'
				study[5] = 'I'
				binary.LittleEndian.PutUint16(study[6:8], uint16(len(uid)))
				data = append(data, study...)
				data = append(data, uid...)

				return data
			}(),
			expectedLen: 2,
			checks: func(t *testing.T, ds *Dataset) {
				uid := ds.GetString(Tag{0x0020, 0x000D})
				if uid != "1.2.3.4.5.6" {
					t.Errorf("Expected StudyInstanceUID 1.2.3.4.5.6, got %q", uid)
				}
			},
		},
		{
			// Regression test: per PS3.5, a VR=UN element with undefined
			// length must have its nested content walked as Implicit VR
			// Little Endian even inside an Explicit VR dataset, since the
			// true VR was unknown to the sender. The nested element here has
			// an implicit-VR length (4) whose low bytes don't form a
			// recognized VR code, so misreading it as Explicit VR desyncs
			// the offset and corrupts/drops the trailing StudyInstanceUID.
			name: "Undefined-length UN parses nested content as Implicit VR",
			data: func() []byte {
				var data []byte

				// (0009,0010) Private tag, VR=UN, undefined length
				un := make([]byte, 12)
				binary.LittleEndian.PutUint16(un[0:2], 0x0009)
				binary.LittleEndian.PutUint16(un[2:4], 0x0010)
				un[4] = 'U'
				un[5] = 'N'
				binary.LittleEndian.PutUint32(un[8:12], undefinedLength)
				data = append(data, un...)

				// Item with undefined length (content is an implicit-VR
				// element stream terminated by an Item Delimitation Item)
				item := make([]byte, 8)
				binary.LittleEndian.PutUint16(item[0:2], 0xFFFE)
				binary.LittleEndian.PutUint16(item[2:4], 0xE000)
				binary.LittleEndian.PutUint32(item[4:8], undefinedLength)
				data = append(data, item...)

				// Nested element, Implicit VR framing: tag(4) + length(4) + payload.
				// Tag (0009,0001), length 4, payload "WXYZ".
				nested := make([]byte, 8)
				binary.LittleEndian.PutUint16(nested[0:2], 0x0009)
				binary.LittleEndian.PutUint16(nested[2:4], 0x0001)
				binary.LittleEndian.PutUint32(nested[4:8], 4)
				data = append(data, nested...)
				data = append(data, []byte("WXYZ")...)

				// Item Delimitation Item, length 0
				itemDelim := make([]byte, 8)
				binary.LittleEndian.PutUint16(itemDelim[0:2], 0xFFFE)
				binary.LittleEndian.PutUint16(itemDelim[2:4], 0xE00D)
				binary.LittleEndian.PutUint32(itemDelim[4:8], 0)
				data = append(data, itemDelim...)

				// Sequence Delimitation Item, length 0
				seqDelim := make([]byte, 8)
				binary.LittleEndian.PutUint16(seqDelim[0:2], 0xFFFE)
				binary.LittleEndian.PutUint16(seqDelim[2:4], 0xE0DD)
				binary.LittleEndian.PutUint32(seqDelim[4:8], 0)
				data = append(data, seqDelim...)

				// (0020,000D) Study Instance UID, VR=UI, short form
				uid := []byte("1.9.8.7.6.5.4")
				if len(uid)%2 == 1 {
					uid = append(uid, 0x00)
				}
				study := make([]byte, 8)
				binary.LittleEndian.PutUint16(study[0:2], 0x0020)
				binary.LittleEndian.PutUint16(study[2:4], 0x000D)
				study[4] = 'U'
				study[5] = 'I'
				binary.LittleEndian.PutUint16(study[6:8], uint16(len(uid)))
				data = append(data, study...)
				data = append(data, uid...)

				return data
			}(),
			expectedLen: 2,
			checks: func(t *testing.T, ds *Dataset) {
				uid := ds.GetString(Tag{0x0020, 0x000D})
				if uid != "1.9.8.7.6.5.4" {
					t.Errorf("Expected StudyInstanceUID 1.9.8.7.6.5.4, got %q", uid)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ds, err := ParseDataset(tt.data)
			if err != nil {
				t.Fatalf("ParseDataset failed: %v", err)
			}

			if len(ds.Elements) != tt.expectedLen {
				t.Errorf("Expected %d elements, got %d", tt.expectedLen, len(ds.Elements))
			}

			if tt.checks != nil {
				tt.checks(t, ds)
			}
		})
	}
}

func TestParseImplicitVRDataset(t *testing.T) {
	// Same regression scenario as TestParseDataset's undefined-length SQ
	// cases, but encoded as Implicit VR Little Endian: no VR field, every
	// element header is tag(4)+length(4). Item/delimiter tags are unaffected
	// by transfer syntax, so the same skipUndefinedLength walk applies.
	data := func() []byte {
		var data []byte

		// (0008,1032) Procedure Code Sequence, implicit VR, undefined length
		sq := make([]byte, 8)
		binary.LittleEndian.PutUint16(sq[0:2], 0x0008)
		binary.LittleEndian.PutUint16(sq[2:4], 0x1032)
		binary.LittleEndian.PutUint32(sq[4:8], undefinedLength)
		data = append(data, sq...)

		itemPayload := []byte("PLACEHOLDER!")
		item := make([]byte, 8)
		binary.LittleEndian.PutUint16(item[0:2], 0xFFFE)
		binary.LittleEndian.PutUint16(item[2:4], 0xE000)
		binary.LittleEndian.PutUint32(item[4:8], uint32(len(itemPayload)))
		data = append(data, item...)
		data = append(data, itemPayload...)

		seqDelim := make([]byte, 8)
		binary.LittleEndian.PutUint16(seqDelim[0:2], 0xFFFE)
		binary.LittleEndian.PutUint16(seqDelim[2:4], 0xE0DD)
		binary.LittleEndian.PutUint32(seqDelim[4:8], 0)
		data = append(data, seqDelim...)

		// (0020,000D) Study Instance UID, implicit VR
		uid := []byte("1.2.840.113619.9.9.9")
		if len(uid)%2 == 1 {
			uid = append(uid, 0x00)
		}
		study := make([]byte, 8)
		binary.LittleEndian.PutUint16(study[0:2], 0x0020)
		binary.LittleEndian.PutUint16(study[2:4], 0x000D)
		binary.LittleEndian.PutUint32(study[4:8], uint32(len(uid)))
		data = append(data, study...)
		data = append(data, uid...)

		return data
	}()

	ds, err := ParseDatasetWithTransferSyntax(data, TransferSyntaxImplicitVRLittleEndian)
	if err != nil {
		t.Fatalf("ParseDatasetWithTransferSyntax failed: %v", err)
	}

	if len(ds.Elements) != 2 {
		t.Errorf("Expected 2 elements, got %d", len(ds.Elements))
	}

	uid := ds.GetString(Tag{0x0020, 0x000D})
	if uid != "1.2.840.113619.9.9.9" {
		t.Errorf("Expected StudyInstanceUID 1.2.840.113619.9.9.9, got %q", uid)
	}
}

func TestDataset_EncodeDataset(t *testing.T) {
	tests := []struct {
		name   string
		setup  func() *Dataset
		verify func(t *testing.T, data []byte)
	}{
		{
			name: "Empty dataset",
			setup: func() *Dataset {
				return NewDataset()
			},
			verify: func(t *testing.T, data []byte) {
				if len(data) != 0 {
					t.Errorf("Expected empty data, got %d bytes", len(data))
				}
			},
		},
		{
			name: "Single element",
			setup: func() *Dataset {
				ds := NewDataset()
				ds.AddElement(Tag{0x0010, 0x0010}, VR_PN, "DOE^JOHN")
				return ds
			},
			verify: func(t *testing.T, data []byte) {
				// Explicit VR format: Tag (4) + VR (2) + Length (2) + Value
				if len(data) < 8 {
					t.Fatalf("Data too short: %d bytes", len(data))
				}

				// Verify tag
				group := binary.LittleEndian.Uint16(data[0:2])
				element := binary.LittleEndian.Uint16(data[2:4])
				if group != 0x0010 || element != 0x0010 {
					t.Errorf("Expected tag (0010,0010), got (%04x,%04x)", group, element)
				}

				// Verify VR
				vr := string(data[4:6])
				if vr != "PN" {
					t.Errorf("Expected VR PN, got %s", vr)
				}

				// Verify length (2 bytes for short VR in Explicit VR)
				length := binary.LittleEndian.Uint16(data[6:8])
				if length != 8 {
					t.Errorf("Expected length 8, got %d", length)
				}

				// Verify value
				value := string(data[8 : 8+length])
				if value != "DOE^JOHN" {
					t.Errorf("Expected DOE^JOHN, got %s", value)
				}
			},
		},
		{
			name: "Element with odd length gets padded",
			setup: func() *Dataset {
				ds := NewDataset()
				ds.AddElement(Tag{0x0010, 0x0010}, VR_PN, "JOHNSON") // 7 bytes (odd)
				return ds
			},
			verify: func(t *testing.T, data []byte) {
				// Explicit VR: Tag (4) + VR (2) + Length (2) + Value
				// Verify length is padded to even
				length := binary.LittleEndian.Uint16(data[6:8])
				if length%2 != 0 {
					t.Errorf("Expected even length, got %d", length)
				}
				if length != 8 { // 7 + 1 padding
					t.Errorf("Expected padded length 8, got %d", length)
				}
			},
		},
		{
			name: "Multiple elements in tag order",
			setup: func() *Dataset {
				ds := NewDataset()
				// Add in reverse order to test sorting
				ds.AddElement(Tag{0x0020, 0x000D}, VR_UI, "1.2.3")
				ds.AddElement(Tag{0x0010, 0x0020}, VR_LO, "12345")
				ds.AddElement(Tag{0x0010, 0x0010}, VR_PN, "DOE^JOHN")
				return ds
			},
			verify: func(t *testing.T, data []byte) {
				// Explicit VR format
				// Verify first tag is smallest (0010,0010)
				group := binary.LittleEndian.Uint16(data[0:2])
				element := binary.LittleEndian.Uint16(data[2:4])
				if group != 0x0010 || element != 0x0010 {
					t.Errorf("First tag should be (0010,0010), got (%04x,%04x)", group, element)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ds := tt.setup()
			data := ds.EncodeDataset()
			tt.verify(t, data)
		})
	}
}

func TestDataset_RoundTrip(t *testing.T) {
	// Create a dataset, encode it, parse it back, and verify
	original := NewDataset()
	original.AddElement(Tag{0x0010, 0x0010}, VR_PN, "DOE^JOHN")
	original.AddElement(Tag{0x0010, 0x0020}, VR_LO, "12345")
	original.AddElement(Tag{0x0008, 0x0060}, VR_CS, "CT")
	original.AddElement(Tag{0x0020, 0x000D}, VR_UI, "1.2.3.4.5")

	// Encode
	encoded := original.EncodeDataset()

	// Parse back
	parsed, err := ParseDataset(encoded)
	if err != nil {
		t.Fatalf("Failed to parse encoded dataset: %v", err)
	}

	// Verify all elements
	tests := []struct {
		tag      Tag
		expected string
	}{
		{Tag{0x0010, 0x0010}, "DOE^JOHN"},
		{Tag{0x0010, 0x0020}, "12345"},
		{Tag{0x0008, 0x0060}, "CT"},
		{Tag{0x0020, 0x000D}, "1.2.3.4.5"},
	}

	for _, tt := range tests {
		value := parsed.GetString(tt.tag)
		if value != tt.expected {
			t.Errorf("Tag %v: expected %q, got %q", tt.tag, tt.expected, value)
		}
	}
}

// TestDataset_RoundTrip_UndefinedLength is a regression test: re-encoding a
// parsed undefined-length element used to write a defined length computed
// from the captured byte count, even though those bytes still contain
// embedded Item/Sequence-Delimitation tag headers. That produces invalid
// DICOM. The length field must round-trip as 0xFFFFFFFF.
func TestDataset_RoundTrip_UndefinedLength(t *testing.T) {
	original := func() []byte {
		var data []byte

		// (0008,1032) Procedure Code Sequence, VR=SQ, undefined length
		sq := make([]byte, 12)
		binary.LittleEndian.PutUint16(sq[0:2], 0x0008)
		binary.LittleEndian.PutUint16(sq[2:4], 0x1032)
		sq[4] = 'S'
		sq[5] = 'Q'
		binary.LittleEndian.PutUint32(sq[8:12], undefinedLength)
		data = append(data, sq...)

		itemPayload := []byte("PLACEHOLDER!")
		item := make([]byte, 8)
		binary.LittleEndian.PutUint16(item[0:2], 0xFFFE)
		binary.LittleEndian.PutUint16(item[2:4], 0xE000)
		binary.LittleEndian.PutUint32(item[4:8], uint32(len(itemPayload)))
		data = append(data, item...)
		data = append(data, itemPayload...)

		seqDelim := make([]byte, 8)
		binary.LittleEndian.PutUint16(seqDelim[0:2], 0xFFFE)
		binary.LittleEndian.PutUint16(seqDelim[2:4], 0xE0DD)
		binary.LittleEndian.PutUint32(seqDelim[4:8], 0)
		data = append(data, seqDelim...)

		uid := []byte("1.2.840.113619.2.1.0")
		if len(uid)%2 == 1 {
			uid = append(uid, 0x00)
		}
		study := make([]byte, 8)
		binary.LittleEndian.PutUint16(study[0:2], 0x0020)
		binary.LittleEndian.PutUint16(study[2:4], 0x000D)
		study[4] = 'U'
		study[5] = 'I'
		binary.LittleEndian.PutUint16(study[6:8], uint16(len(uid)))
		data = append(data, study...)
		data = append(data, uid...)

		return data
	}()

	parsed, err := ParseDataset(original)
	if err != nil {
		t.Fatalf("ParseDataset failed: %v", err)
	}

	encoded := parsed.EncodeDataset()

	// The SQ element sorts first (group 0008 < 0020), so its 12-byte
	// long-VR header starts at offset 0: tag(4) + VR(2) + reserved(2) + length(4).
	length := binary.LittleEndian.Uint32(encoded[8:12])
	if length != undefinedLength {
		t.Errorf("Expected re-encoded length to be 0xFFFFFFFF, got 0x%X", length)
	}

	reparsed, err := ParseDataset(encoded)
	if err != nil {
		t.Fatalf("Re-parsing re-encoded dataset failed: %v", err)
	}
	uid := reparsed.GetString(Tag{0x0020, 0x000D})
	if uid != "1.2.840.113619.2.1.0" {
		t.Errorf("Expected StudyInstanceUID 1.2.840.113619.2.1.0 after round-trip, got %q", uid)
	}
}

// TestDataset_RoundTrip_UndefinedLength_ImplicitVR mirrors
// TestDataset_RoundTrip_UndefinedLength for the Implicit VR encode/parse path.
func TestDataset_RoundTrip_UndefinedLength_ImplicitVR(t *testing.T) {
	original := func() []byte {
		var data []byte

		sq := make([]byte, 8)
		binary.LittleEndian.PutUint16(sq[0:2], 0x0008)
		binary.LittleEndian.PutUint16(sq[2:4], 0x1032)
		binary.LittleEndian.PutUint32(sq[4:8], undefinedLength)
		data = append(data, sq...)

		itemPayload := []byte("PLACEHOLDER!")
		item := make([]byte, 8)
		binary.LittleEndian.PutUint16(item[0:2], 0xFFFE)
		binary.LittleEndian.PutUint16(item[2:4], 0xE000)
		binary.LittleEndian.PutUint32(item[4:8], uint32(len(itemPayload)))
		data = append(data, item...)
		data = append(data, itemPayload...)

		seqDelim := make([]byte, 8)
		binary.LittleEndian.PutUint16(seqDelim[0:2], 0xFFFE)
		binary.LittleEndian.PutUint16(seqDelim[2:4], 0xE0DD)
		binary.LittleEndian.PutUint32(seqDelim[4:8], 0)
		data = append(data, seqDelim...)

		uid := []byte("1.2.840.113619.9.9.9")
		if len(uid)%2 == 1 {
			uid = append(uid, 0x00)
		}
		study := make([]byte, 8)
		binary.LittleEndian.PutUint16(study[0:2], 0x0020)
		binary.LittleEndian.PutUint16(study[2:4], 0x000D)
		binary.LittleEndian.PutUint32(study[4:8], uint32(len(uid)))
		data = append(data, study...)
		data = append(data, uid...)

		return data
	}()

	parsed, err := ParseDatasetWithTransferSyntax(original, TransferSyntaxImplicitVRLittleEndian)
	if err != nil {
		t.Fatalf("ParseDatasetWithTransferSyntax failed: %v", err)
	}

	encoded, err := EncodeDatasetWithTransferSyntax(parsed, TransferSyntaxImplicitVRLittleEndian)
	if err != nil {
		t.Fatalf("EncodeDatasetWithTransferSyntax failed: %v", err)
	}

	// Implicit VR header: tag(4) + length(4). SQ tag sorts first.
	length := binary.LittleEndian.Uint32(encoded[4:8])
	if length != undefinedLength {
		t.Errorf("Expected re-encoded length to be 0xFFFFFFFF, got 0x%X", length)
	}

	reparsed, err := ParseDatasetWithTransferSyntax(encoded, TransferSyntaxImplicitVRLittleEndian)
	if err != nil {
		t.Fatalf("Re-parsing re-encoded dataset failed: %v", err)
	}
	uid := reparsed.GetString(Tag{0x0020, 0x000D})
	if uid != "1.2.840.113619.9.9.9" {
		t.Errorf("Expected StudyInstanceUID 1.2.840.113619.9.9.9 after round-trip, got %q", uid)
	}
}

func TestDetermineVR(t *testing.T) {
	tests := []struct {
		name     string
		tag      Tag
		expected string
	}{
		{"Patient Name", Tag{0x0010, 0x0010}, VR_PN},
		{"Patient ID", Tag{0x0010, 0x0020}, VR_LO},
		{"Study Instance UID", Tag{0x0020, 0x000D}, VR_UI},
		{"Series Instance UID", Tag{0x0020, 0x000E}, VR_UI},
		{"Modality", Tag{0x0008, 0x0060}, VR_CS},
		{"Study Date", Tag{0x0008, 0x0020}, VR_DA},
		{"Specific Character Set", Tag{0x0008, 0x0005}, VR_CS},
		{"SOP Class UID", Tag{0x0008, 0x0016}, VR_UI},
		{"SOP Instance UID", Tag{0x0008, 0x0018}, VR_UI},
		{"Study Time", Tag{0x0008, 0x0030}, VR_TM},
		{"Accession Number", Tag{0x0008, 0x0050}, VR_SH},
		{"Query/Retrieve Level", Tag{0x0008, 0x0052}, VR_CS},
		{"Institution Name", Tag{0x0008, 0x0080}, VR_LO},
		{"Referring Physician", Tag{0x0008, 0x0090}, VR_PN},
		{"Study Description", Tag{0x0008, 0x1030}, VR_LO},
		{"Patient Birth Date", Tag{0x0010, 0x0030}, VR_DA},
		{"Patient Sex", Tag{0x0010, 0x0040}, VR_CS},
		{"Patient Age", Tag{0x0010, 0x1010}, VR_AS},
		{"Study ID", Tag{0x0020, 0x0010}, VR_SH},
		{"Series Number", Tag{0x0020, 0x0011}, VR_IS},
		{"Instance Number", Tag{0x0020, 0x0013}, VR_IS},
		{"Unknown tag", Tag{0xFFFF, 0xFFFF}, VR_UN},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := determineVR(tt.tag)
			if result != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestEncodeElementValue_VariousTypes(t *testing.T) {
	tests := []struct {
		name    string
		element *Element
		checkFn func(t *testing.T, result []byte)
	}{
		{
			name: "String value",
			element: &Element{
				Tag:   Tag{0x0010, 0x0010},
				VR:    VR_PN,
				Value: "DOE^JOHN",
			},
			checkFn: func(t *testing.T, result []byte) {
				if string(result) != "DOE^JOHN" {
					t.Errorf("Expected DOE^JOHN, got %s", string(result))
				}
			},
		},
		{
			name: "String with null terminator",
			element: &Element{
				Tag:   Tag{0x0010, 0x0020},
				VR:    VR_LO,
				Value: "12345\x00\x00",
			},
			checkFn: func(t *testing.T, result []byte) {
				if string(result) != "12345" {
					t.Errorf("Expected 12345, got %s", string(result))
				}
			},
		},
		{
			name: "String array",
			element: &Element{
				Tag:   Tag{0x0008, 0x0060},
				VR:    VR_CS,
				Value: []string{"CT", "MR"},
			},
			checkFn: func(t *testing.T, result []byte) {
				if string(result) != "CT\\MR" {
					t.Errorf("Expected CT\\MR, got %s", string(result))
				}
			},
		},
		{
			name: "Integer value",
			element: &Element{
				Tag:   Tag{0x0020, 0x0013},
				VR:    VR_IS,
				Value: 42,
			},
			checkFn: func(t *testing.T, result []byte) {
				if string(result) != "42" {
					t.Errorf("Expected 42, got %s", string(result))
				}
			},
		},
		{
			name: "Uint16 value",
			element: &Element{
				Tag:   Tag{0x0000, 0x0100},
				VR:    VR_US,
				Value: uint16(0x0020),
			},
			checkFn: func(t *testing.T, result []byte) {
				if len(result) != 2 || result[0] != 0x20 || result[1] != 0x00 {
					t.Errorf("Expected [0x20, 0x00], got %v", result)
				}
			},
		},
		{
			name: "Uint32 value",
			element: &Element{
				Tag:   Tag{0x0000, 0x1000},
				VR:    VR_UL,
				Value: uint32(0x12345678),
			},
			checkFn: func(t *testing.T, result []byte) {
				if len(result) != 4 {
					t.Errorf("Expected 4 bytes, got %d", len(result))
				}
			},
		},
		{
			name: "Other type (float)",
			element: &Element{
				Tag:   Tag{0x0010, 0x0010},
				VR:    VR_LO,
				Value: 3.14159,
			},
			checkFn: func(t *testing.T, result []byte) {
				if len(result) == 0 {
					t.Error("Expected non-empty result")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := encodeElementValue(tt.element)
			tt.checkFn(t, result)
		})
	}
}
