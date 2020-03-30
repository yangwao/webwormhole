// Command cpace-machine is a netcat-like pipe over WebRTC.
//
// WebRTC uses DTLS-RSTP (https://tools.ietf.org/html/rfc5764) to secure its
// data. The mechanism it uses to securely exchange keys relies on exchanging
// metadata that includes both endpoints' certificate fingerprints via some
// trusted channel, typically a signalling server over https and websockets.
// More in RFC5763 (https://tools.ietf.org/html/rfc5763).
//
// This program is an attempt to remove the signalling server from the trust
// model by using a PAKE to estabish the authenticity of the WebRTC metadata.
// In other words, it's a clone of Magic Wormhole made to use WebRTC as the
// transport.
//
// The handshake needs a signalling server that facilitates exchanging arbitrary
// messages via a slot system. The package minsig implements such a server.
//
// Rough sketch of the handshake:
//
//	A								S								B
//	  ----PUT /slot if-match:0--->
//			pake_msg_a(A)
//										<---PUT /slot if-match:0---
//												pake_msg_a(B)
//										--status:Conflict etag:X-->
//												pake_msg_a(A)
//										<---PUT /slot if-match:X---
//											pake_msg_b(B)+sbox(offer sdp)
//	  <-----status:OK etag:X------
//			pake_msg_b+sbox(off)
//	  --DELETE /slot if-match:X-->
//			sbox(answer sdp)
//										---status:OK etag:X------->
//												sbox(answer sdp)
package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
)

/*
thoughts on metadata
	?filetype
	?size
	?name

no-metadata stream is also nice to keep as an option

integrity check?

resumption
	offset + checksum
	fancier? rsync-style chunks + rolling checksum?

simple header, stream secretboxes
*/

func usage() {
	fmt.Fprintf(flag.CommandLine.Output(), `usage: %#[1]s <public slot> <secret password>

cpace-machine creates secure ephemeral pipes between computers. if
the slot and password are the same in two invocations, they will
be connected.

flags:
`, os.Args[0])
	flag.PrintDefaults()
}

func main() {
	flag.Usage = usage
	iceserv := flag.String("ice", "stun:stun.l.google.com:19302", "stun or turn servers to use")
	sigserv := flag.String("minsig", "https://minimumsignal.0f.io/", "signalling server to use")
	flag.Parse()
	if flag.NArg() != 2 {
		flag.Usage()
		os.Exit(-1)
	}
	slot := flag.Arg(0)
	pass := flag.Arg(1)

	// TODO generate and print slots and passwords
	// TODO use pgp words for code
	// TODO (optionally) ask for confirmation before moving data
	c, err := Dial(slot, pass, *sigserv, strings.Split(*iceserv, ","))
	if err != nil {
		log.Fatalf("could not dial: %v", err)
	}

	done := make(chan struct{})
	// The recieve end of the pipe.
	go func() {
		_, err := io.Copy(os.Stdout, c)
		if err != nil {
			log.Printf("could not write to stdout: %v", err)
		}
		//log.Printf("debug: rx %v", n)
		done <- struct{}{}
	}()
	// The send end of the pipe.
	go func() {
		_, err := io.Copy(c, os.Stdin)
		if err != nil {
			log.Printf("could not write to channel: %v", err)
		}
		//log.Printf("debug: tx %v", n)
		done <- struct{}{}
	}()
	<-done
	c.Close()
}