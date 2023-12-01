package derive

import (
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"

	"github.com/khulnasoft-lab/tracker/pkg/events"
	"github.com/khulnasoft-lab/tracker/types/trace"
)

func NetPacketIPv6() DeriveFunction {
	return deriveSingleEvent(events.NetPacketIPv6, deriveNetPacketIPv6Args())
}

func deriveNetPacketIPv6Args() deriveArgsFunction {
	return func(event trace.Event) ([]interface{}, error) {
		// event retval encodes layer 3 protocol type

		if event.ReturnValue&familyIpv6 != familyIpv6 {
			return nil, nil
		}

		payload, err := parsePayloadArg(&event)
		if err != nil {
			return nil, err
		}

		// parse packet

		packet := gopacket.NewPacket(
			payload,
			layers.LayerTypeIPv6,
			gopacket.Default,
		)
		if packet == nil {
			return []interface{}{}, parsePacketError()
		}

		layer3 := packet.NetworkLayer()

		switch l3 := layer3.(type) {
		case (*layers.IPv6):
			var ipv6 trace.ProtoIPv6
			copyIPv6ToProtoIPv6(l3, &ipv6)
			md := trace.PacketMetadata{
				Direction: getPacketDirection(&event),
			}

			return []interface{}{
				l3.SrcIP.String(),
				l3.DstIP.String(),
				md,
				ipv6,
			}, nil
		}

		return nil, notProtoPacketError("IPv6")
	}
}

//
// IPv6 protocol type conversion (from gopacket layer to trace type)
//

func copyIPv6ToProtoIPv6(l3 *layers.IPv6, proto *trace.ProtoIPv6) {
	proto.Version = l3.Version
	proto.TrafficClass = l3.TrafficClass
	proto.FlowLabel = l3.FlowLabel
	proto.Length = l3.Length
	proto.NextHeader = l3.NextHeader.String()
	proto.HopLimit = l3.HopLimit
	proto.SrcIP = l3.SrcIP.String()
	proto.DstIP = l3.DstIP.String()
}
