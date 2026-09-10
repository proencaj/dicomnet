package client

import (
	"encoding/binary"
	"io"
	"log/slog"
	"testing"

	"github.com/caio-sobreiro/dicomnet/pdu"
	"github.com/caio-sobreiro/dicomnet/types"
)

// acCtx describes a presentation context to encode into a synthetic
// A-ASSOCIATE-AC PDU.
type acCtx struct {
	id     byte
	result byte
	ts     string
}

func appendLen16(buf []byte, n int) []byte {
	b := make([]byte, 2)
	binary.BigEndian.PutUint16(b, uint16(n))
	return append(buf, b...)
}

// buildAC hand-crafts an A-ASSOCIATE-AC PDU (including the 6-byte PDU header).
func buildAC(contexts []acCtx, maxPDU uint32, includeUserInfo bool) []byte {
	body := make([]byte, 68) // fixed fields, contents irrelevant to the parser

	appUID := []byte(types.ApplicationContextUID)
	body = append(body, 0x10, 0x00)
	body = appendLen16(body, len(appUID))
	body = append(body, appUID...)

	for _, c := range contexts {
		var sub []byte
		if c.ts != "" {
			sub = append(sub, 0x40, 0x00)
			sub = appendLen16(sub, len(c.ts))
			sub = append(sub, []byte(c.ts)...)
		}
		item := []byte{0x21, 0x00}
		item = appendLen16(item, 4+len(sub))
		// PS3.8 Table 9-18: context ID, reserved, Result/Reason, reserved.
		item = append(item, c.id, 0x00, c.result, 0x00)
		item = append(item, sub...)
		body = append(body, item...)
	}

	if includeUserInfo {
		ui := []byte{0x51, 0x00, 0x00, 0x04}
		v := make([]byte, 4)
		binary.BigEndian.PutUint32(v, maxPDU)
		ui = append(ui, v...)
		item := []byte{0x50, 0x00}
		item = appendLen16(item, len(ui))
		item = append(item, ui...)
		body = append(body, item...)
	}

	hdr := []byte{pdu.TypeAssociateAC, 0x00}
	hdr = binary.BigEndian.AppendUint32(hdr, uint32(len(body)))
	return append(hdr, body...)
}

func newTestAssoc(configuredMaxPDU uint32, ctxs map[byte]*PresentationContext) (*Association, *mockConn) {
	conn := newMockConn()
	return &Association{
		conn:             conn,
		callingAETitle:   "SCU",
		calledAETitle:    "SCP",
		maxPDULength:     configuredMaxPDU,
		presentationCtxs: ctxs,
		logger:           slog.New(slog.NewTextHandler(io.Discard, nil)),
	}, conn
}

func TestReceiveAssociateAC(t *testing.T) {
	const (
		ctSOP = "1.2.840.10008.5.1.4.1.1.2"
		mrSOP = "1.2.840.10008.5.1.4.1.1.4"
		scSOP = "1.2.840.10008.5.1.4.1.1.7"
	)

	t.Run("all contexts accepted", func(t *testing.T) {
		assoc, conn := newTestAssoc(16384, map[byte]*PresentationContext{
			1: {ID: 1, AbstractSyntax: ctSOP},
			3: {ID: 3, AbstractSyntax: mrSOP},
		})
		conn.readBuf.Write(buildAC([]acCtx{
			{id: 1, result: 0, ts: types.ExplicitVRLittleEndian},
			{id: 3, result: 0, ts: types.ImplicitVRLittleEndian},
		}, 0, false))

		if err := assoc.receiveAssociateAC(); err != nil {
			t.Fatalf("receiveAssociateAC: %v", err)
		}

		if !assoc.presentationCtxs[1].Accepted || assoc.presentationCtxs[1].TransferSyntax != types.ExplicitVRLittleEndian {
			t.Errorf("ctx 1 = %+v", assoc.presentationCtxs[1])
		}
		if !assoc.presentationCtxs[3].Accepted || assoc.presentationCtxs[3].TransferSyntax != types.ImplicitVRLittleEndian {
			t.Errorf("ctx 3 = %+v", assoc.presentationCtxs[3])
		}
	})

	t.Run("mix of accepted and rejected", func(t *testing.T) {
		assoc, conn := newTestAssoc(16384, map[byte]*PresentationContext{
			1: {ID: 1, AbstractSyntax: ctSOP},
			3: {ID: 3, AbstractSyntax: mrSOP},
			5: {ID: 5, AbstractSyntax: scSOP},
		})
		conn.readBuf.Write(buildAC([]acCtx{
			{id: 1, result: 0, ts: types.ExplicitVRLittleEndian},
			// rejected - abstract syntax not supported (3), still carries a bogus TS
			{id: 3, result: 3, ts: types.ExplicitVRLittleEndian},
			// rejected - transfer syntaxes not supported (4)
			{id: 5, result: 4},
		}, 0, false))

		if err := assoc.receiveAssociateAC(); err != nil {
			t.Fatalf("receiveAssociateAC: %v", err)
		}

		if !assoc.presentationCtxs[1].Accepted || assoc.presentationCtxs[1].TransferSyntax != types.ExplicitVRLittleEndian {
			t.Errorf("ctx 1 should be unaffected: %+v", assoc.presentationCtxs[1])
		}
		for _, id := range []byte{3, 5} {
			pc := assoc.presentationCtxs[id]
			if pc.Accepted {
				t.Errorf("ctx %d should be rejected: %+v", id, pc)
			}
			if pc.TransferSyntax != "" {
				t.Errorf("ctx %d should have empty TransferSyntax, got %q", id, pc.TransferSyntax)
			}
		}

		if _, err := assoc.GetPresentationContextID(mrSOP); err == nil {
			t.Error("GetPresentationContextID should fail for a rejected context")
		}
	})

	t.Run("user info announces smaller max PDU", func(t *testing.T) {
		assoc, conn := newTestAssoc(16384, map[byte]*PresentationContext{
			1: {ID: 1, AbstractSyntax: ctSOP},
		})
		conn.readBuf.Write(buildAC([]acCtx{
			{id: 1, result: 0, ts: types.ExplicitVRLittleEndian},
		}, 8192, true))

		if err := assoc.receiveAssociateAC(); err != nil {
			t.Fatalf("receiveAssociateAC: %v", err)
		}
		if assoc.maxPDULength != 8192 {
			t.Errorf("maxPDULength = %d, want 8192", assoc.maxPDULength)
		}
	})

	t.Run("user info announces unlimited max PDU keeps configured", func(t *testing.T) {
		assoc, conn := newTestAssoc(16384, map[byte]*PresentationContext{
			1: {ID: 1, AbstractSyntax: ctSOP},
		})
		conn.readBuf.Write(buildAC([]acCtx{
			{id: 1, result: 0, ts: types.ExplicitVRLittleEndian},
		}, 0, true))

		if err := assoc.receiveAssociateAC(); err != nil {
			t.Fatalf("receiveAssociateAC: %v", err)
		}
		if assoc.maxPDULength != 16384 {
			t.Errorf("maxPDULength = %d, want 16384 (configured)", assoc.maxPDULength)
		}
	})

	t.Run("peer announces larger max PDU keeps configured", func(t *testing.T) {
		assoc, conn := newTestAssoc(16384, map[byte]*PresentationContext{
			1: {ID: 1, AbstractSyntax: ctSOP},
		})
		conn.readBuf.Write(buildAC([]acCtx{
			{id: 1, result: 0, ts: types.ExplicitVRLittleEndian},
		}, 65536, true))

		if err := assoc.receiveAssociateAC(); err != nil {
			t.Fatalf("receiveAssociateAC: %v", err)
		}
		if assoc.maxPDULength != 16384 {
			t.Errorf("maxPDULength = %d, want 16384 (configured)", assoc.maxPDULength)
		}
	})
}

type stubDIMSEHandler struct{}

func (stubDIMSEHandler) HandleDIMSEMessage(byte, byte, []byte, *pdu.Layer) error { return nil }

// TestAssociateAC_RoundTrip guards against the server AC writer and the client
// AC parser drifting apart: the client builds an A-ASSOCIATE-RQ, the server
// negotiates it and writes an A-ASSOCIATE-AC, and the client parses that AC.
func TestAssociateAC_RoundTrip(t *testing.T) {
	const ctSOP = "1.2.840.10008.5.1.4.1.1.2"
	discard := slog.New(slog.NewTextHandler(io.Discard, nil))

	// Client side: build and "send" the A-ASSOCIATE-RQ.
	clientConn := newMockConn()
	assoc := &Association{
		conn:                      clientConn,
		callingAETitle:            "SCU",
		calledAETitle:             "SCP",
		maxPDULength:              8192,
		presentationCtxs:          make(map[byte]*PresentationContext),
		logger:                    discard,
		preferredTransferSyntaxes: []string{types.ExplicitVRLittleEndian, types.ImplicitVRLittleEndian},
		sopClasses:                []string{ctSOP},
	}
	if err := assoc.sendAssociateRQ(); err != nil {
		t.Fatalf("sendAssociateRQ: %v", err)
	}
	rq := clientConn.writeBuf.Bytes()

	// Server side: consume the RQ and emit the AC via the real writer.
	serverConn := newMockConn()
	serverConn.readBuf.Write(rq)
	layer := pdu.NewLayer(serverConn, stubDIMSEHandler{}, "SCP", discard)
	_ = layer.HandleConnection() // returns after the RQ, then EOF on the empty read buffer
	ac := serverConn.writeBuf.Bytes()
	if len(ac) == 0 {
		t.Fatal("server wrote no A-ASSOCIATE-AC")
	}

	// Client side: parse the AC the server produced.
	clientConn.readBuf.Write(ac)
	if err := assoc.receiveAssociateAC(); err != nil {
		t.Fatalf("receiveAssociateAC: %v", err)
	}

	pc := assoc.presentationCtxs[1]
	if pc == nil || !pc.Accepted {
		t.Fatalf("ctx 1 not accepted: %+v", pc)
	}
	if pc.TransferSyntax != types.ExplicitVRLittleEndian {
		t.Errorf("ctx 1 TransferSyntax = %q, want %q", pc.TransferSyntax, types.ExplicitVRLittleEndian)
	}
	if id, err := assoc.GetPresentationContextID(ctSOP); err != nil || id != 1 {
		t.Errorf("GetPresentationContextID = %d, %v; want 1, nil", id, err)
	}
	if assoc.maxPDULength != 8192 {
		t.Errorf("maxPDULength = %d, want 8192", assoc.maxPDULength)
	}
}
