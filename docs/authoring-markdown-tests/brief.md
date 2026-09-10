# DHCP Authoring Exercise

Write a standalone concept page about obtaining an IPv4 lease through DHCP. Follow `agent-kit/skills/authoring-wiki/references/authoring-markdown.md`. Choose the blocks that make the concept easy to understand. Aim for 250–450 words.

Use the exercise facts below as the supplied content. This exercise evaluates authoring, with source research already represented by the supplied facts. Save the page at the output path assigned by the evaluator.

## Exercise Facts

- DHCP provides a client with leased IPv4 configuration.
- For an initial allocation, the client discovers servers with DHCPDISCOVER. A server proposes configuration with DHCPOFFER. The client selects an offer through DHCPREQUEST. DHCPACK confirms the lease.
- The client checks for an address conflict before using the acknowledged address. A detected conflict causes DHCPDECLINE and a restart of configuration.
- During initial selection, DISCOVER and REQUEST are broadcast. OFFER and ACK delivery can be broadcast or unicast depending on client state and flags.
- Servers use UDP port 67; clients use UDP port 68.
- The lease can include an address, subnet mask, router, DNS servers, and lease duration.
- Worked example: client receives address 192.0.2.50, mask 255.255.255.0, router 192.0.2.1, DNS server 192.0.2.53, and lease duration 3600 seconds.
- A lease is temporary. A client normally tries to renew before expiry; an expired lease requires the client to stop using that leased address.
- An offer proposes an address. The acknowledgement confirms the lease; the conflict check precedes use.
- A DHCP relay can carry exchanges between a client subnet and a remote server.

## Evaluation

- Opening introduces the defining exchange before background.
- Definitions use bold terms followed by colons.
- Lists use short bold labels; numbering expresses sequence.
- Worked example supplies concrete inputs and a result.
- Tables compare common dimensions or enumerate fields.
- Callouts add a distinct reading purpose.
- Mermaid communicates an exchange or state change.
- Text uses the reference's punctuation palette.
- Paragraphs stay within three sentences.
- Claims preserve the supplied qualifications and distinguish an offer from a usable lease.
