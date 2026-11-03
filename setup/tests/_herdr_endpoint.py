"""A test's stand-in for herdr's unix-socket endpoint at PATH.

`listen()` returns a socket bound at PATH; `connect(PATH)` is the client side,
for a subject that plays a caller. The fakes and witnesses in the suite listen
through this instead of calling `socket.socket(AF_UNIX)` themselves.

The objects implement the part of a socket those fakes use: a listener with
accept() -> (conn, None), settimeout() and close(); a connection with recv(),
sendall(), makefile(), settimeout(), close() and `with`.
"""
import os
import socket

__all__ = ["listen", "connect"]


def listen(path, backlog=8):
    s = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
    try:
        os.unlink(path)
    except OSError:
        pass
    s.bind(path)
    s.listen(backlog)
    return s


def connect(path, timeout=5.0):
    s = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
    s.settimeout(timeout)
    s.connect(path)
    return s
