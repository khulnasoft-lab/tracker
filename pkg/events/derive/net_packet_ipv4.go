package derive

import (
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"

	"github.com/khulnasoft-lab/tracker/pkg/events"
	"github.com/khulnasoft-lab/tracker/types/trace"
)

func NetPacketIPv4() DeriveFunction {
	return deriveSingleEvent(events.NetPacketIPv4, deriveNetPacketIPv4Args())
}

func deriveNetPacketIPv4Args() deriveArgsFunction {
	return func(event trace.Event) ([]interface{}, error) {
		// event retval encodes layer 3 protocol type
		if event.ReturnValue&familyIpv4 != familyIpv4 {
			return nil, nil
		}

		payload, err := parsePayloadArg(&event)
		if err != nil {
			return nil, err
		}

		// parse packet

		packet := gopacket.NewPacket(
			payload,
			layers.LayerTypeIPv4,
			gopacket.Default,
		)
		if packet == nil {
			return []interface{}{}, parsePacketError()
		}

		layer3 := packet.NetworkLayer()

		switch l3 := layer3.(type) {
		case (*layers.IPv4):
			var ipv4 trace.ProtoIPv4
			copyIPv4ToProtoIPv4(l3, &ipv4)
			md := trace.PacketMetadata{
				Direction: getPacketDirection(&event),
			}

			return []interface{}{
				l3.SrcIP.String(),
				l3.DstIP.String(),
				md,
				ipv4,
			}, nil
		}

		return nil, notProtoPacketError("IPv4")
	}
}

//
// IPv4 protocol type conversion (from gopacket layer to trace type)
//

func copyIPv4ToProtoIPv4(l3 *layers.IPv4, proto *trace.ProtoIPv4) {
	proto.Version = l3.Version
	proto.IHL = l3.IHL
	proto.TOS = l3.TOS
	proto.Length = l3.Length
	proto.Id = l3.Id
	proto.Flags = uint8(l3.Flags)
	proto.FragOffset = l3.FragOffset
	proto.TTL = l3.TTL
	proto.Protocol = l3.Protocol.String()
	proto.Checksum = l3.Checksum
	proto.SrcIP = l3.SrcIP.String()
	proto.DstIP = l3.DstIP.String()
	// TODO: IPv4 options if IHL > 5
}
