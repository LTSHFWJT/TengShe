package dnstransport

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"

	"golang.org/x/net/dns/dnsmessage"
)

type dnsQuery struct {
	ID       uint16
	Question dnsmessage.Question
	Payload  []byte
}

func buildQuery(domain string, payload []byte, config Config) ([]byte, uint16, error) {
	config = normalizeConfig(config)
	name, err := encodeQueryName(payload, domain, config)
	if err != nil {
		return nil, 0, err
	}
	qname, err := dnsmessage.NewName(name)
	if err != nil {
		return nil, 0, err
	}
	id := randomUint16()
	msg := dnsmessage.Message{
		Header: dnsmessage.Header{
			ID:               id,
			RecursionDesired: true,
		},
		Questions: []dnsmessage.Question{{
			Name:  qname,
			Type:  dnsmessage.TypeTXT,
			Class: dnsmessage.ClassINET,
		}},
	}
	if config.EDNS0 {
		optHeader := dnsmessage.ResourceHeader{Name: dnsmessage.MustNewName(".")}
		if err := optHeader.SetEDNS0(config.EDNS0PayloadSize, dnsmessage.RCodeSuccess, false); err == nil {
			msg.Additionals = []dnsmessage.Resource{{
				Header: optHeader,
				Body:   &dnsmessage.OPTResource{},
			}}
		}
	}
	raw, err := msg.Pack()
	return raw, id, err
}

func parseQuery(raw []byte, domain string) (dnsQuery, error) {
	query, err := parseBasicQuery(raw)
	if err != nil {
		return dnsQuery{}, err
	}
	if query.Question.Type != dnsmessage.TypeTXT {
		return dnsQuery{}, fmt.Errorf("unsupported DNS query type %v", query.Question.Type)
	}
	payload, err := decodeQueryName(query.Question.Name.String(), domain)
	if err != nil {
		return dnsQuery{}, err
	}
	query.Payload = payload
	return query, nil
}

func parseBasicQuery(raw []byte) (dnsQuery, error) {
	var msg dnsmessage.Message
	if err := msg.Unpack(raw); err != nil {
		return dnsQuery{}, err
	}
	if msg.Header.Response {
		return dnsQuery{}, fmt.Errorf("DNS packet is a response")
	}
	if len(msg.Questions) == 0 {
		return dnsQuery{}, fmt.Errorf("DNS query has no question")
	}
	return dnsQuery{
		ID:       msg.Header.ID,
		Question: msg.Questions[0],
	}, nil
}

func buildResponse(query dnsQuery, payload []byte, config Config) ([]byte, error) {
	msg := dnsmessage.Message{
		Header: dnsmessage.Header{
			ID:            query.ID,
			Response:      true,
			Authoritative: true,
			RCode:         dnsmessage.RCodeSuccess,
		},
		Questions: []dnsmessage.Question{query.Question},
		Answers: []dnsmessage.Resource{{
			Header: dnsmessage.ResourceHeader{
				Name:  query.Question.Name,
				Type:  dnsmessage.TypeTXT,
				Class: dnsmessage.ClassINET,
				TTL:   normalizeConfig(config).TTL,
			},
			Body: &dnsmessage.TXTResource{TXT: encodeTextPayload(payload)},
		}},
	}
	return msg.Pack()
}

func buildNXDOMAIN(query dnsQuery) ([]byte, error) {
	msg := dnsmessage.Message{
		Header: dnsmessage.Header{
			ID:            query.ID,
			Response:      true,
			Authoritative: true,
			RCode:         dnsmessage.RCodeNameError,
		},
		Questions: []dnsmessage.Question{query.Question},
	}
	return msg.Pack()
}

func parseResponse(raw []byte, wantID uint16) ([]byte, error) {
	var msg dnsmessage.Message
	if err := msg.Unpack(raw); err != nil {
		return nil, err
	}
	if !msg.Header.Response {
		return nil, fmt.Errorf("DNS packet is not a response")
	}
	if msg.Header.ID != wantID {
		return nil, fmt.Errorf("DNS response id mismatch")
	}
	if msg.Header.RCode != dnsmessage.RCodeSuccess {
		return nil, fmt.Errorf("DNS response rcode %v", msg.Header.RCode)
	}
	for _, answer := range msg.Answers {
		if answer.Header.Type != dnsmessage.TypeTXT {
			continue
		}
		txt, ok := answer.Body.(*dnsmessage.TXTResource)
		if !ok {
			continue
		}
		return decodeTextPayload(txt.TXT)
	}
	return nil, fmt.Errorf("DNS response has no TXT payload")
}

func randomUint16() uint16 {
	var b [2]byte
	if _, err := rand.Read(b[:]); err != nil {
		return 1
	}
	id := binary.BigEndian.Uint16(b[:])
	if id == 0 {
		return 1
	}
	return id
}
