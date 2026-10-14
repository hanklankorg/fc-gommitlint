#!/usr/bin/env python3
# SPDX-FileCopyrightText: The Itiquette Authors
# SPDX-License-Identifier: CC0-1.0

"""Relay one QEMU guestfwd connection to the local metadata server."""

import os
import selectors
import socket
import sys


def relay(port):
    with socket.create_connection(("127.0.0.1", port), timeout=5) as peer:
        with selectors.DefaultSelector() as selector:
            selector.register(0, selectors.EVENT_READ)
            selector.register(peer, selectors.EVENT_READ)
            while events := selector.select(timeout=30):
                for key, _ in events:
                    data = os.read(key.fd, 65536)
                    if key.fileobj is peer:
                        if not data:
                            # End the guest connection as soon as HTTP closes,
                            # even if the guest still has its write side open.
                            return
                        sys.stdout.buffer.write(data)
                        sys.stdout.buffer.flush()
                    elif data:
                        peer.sendall(data)
                    else:
                        selector.unregister(0)
                        peer.shutdown(socket.SHUT_WR)


if __name__ == "__main__":
    relay(int(sys.argv[1]))
