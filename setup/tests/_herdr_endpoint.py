"""A test's stand-in for herdr's endpoint, on every platform bin/herdr-rpc dials.

POSIX: a unix socket bound at PATH — `listen()` returns the socket itself.
Native Windows: the named pipe herdr serves there, named exactly as
bin/herdr-rpc names it (`\\\\.\\pipe\\` + the path's Windows spelling, with
backslashes). CPython has no AF_UNIX on Windows, so the fakes and witnesses in
the suite listen through this instead of `socket.socket(AF_UNIX)`.

On Windows `listen()` also leaves an empty file at PATH once the pipe is up, so
a shell can wait for the endpoint with `[ -e PATH ]` on every platform.

The pipe objects implement the part of a socket those fakes use: a listener with
accept() -> (conn, None), settimeout() and close(); a connection with recv(),
sendall(), makefile(), settimeout(), close() and `with`. `connect(PATH)` is the
client side, for a subject that plays a caller.
"""
import os
import socket
import time

__all__ = ["listen", "connect", "pipe_name"]


def pipe_name(path):
    p = path
    if p.replace("/", "\\").lower().startswith("\\\\.\\pipe\\"):
        return p.replace("/", "\\")
    try:
        from sitecustomize import to_native  # bin/sitecustomize.py, under Git Bash
    except ImportError:
        def to_native(x):
            return x
    return "\\\\.\\pipe\\" + to_native(p).replace("/", "\\")


def listen(path, backlog=8):
    if os.name != "nt":
        s = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
        try:
            os.unlink(path)
        except OSError:
            pass
        s.bind(path)
        s.listen(backlog)
        return s
    lst = _PipeListener(pipe_name(path))
    # A pipe is not a file, so a caller waiting for `[ -e PATH ]` would wait
    # forever: an empty placeholder says the endpoint is listening.
    open(path, "w").close()
    return lst


def connect(path, timeout=5.0):
    if os.name != "nt":
        s = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
        s.settimeout(timeout)
        s.connect(path)
        return s
    import _winapi
    name = pipe_name(path)
    deadline = time.monotonic() + timeout
    while True:
        try:
            h = _winapi.CreateFile(name, _winapi.GENERIC_READ | _winapi.GENERIC_WRITE, 0,
                                   _winapi.NULL, _winapi.OPEN_EXISTING,
                                   _winapi.FILE_FLAG_OVERLAPPED, _winapi.NULL)
            c = _PipeConn(h)
            c.settimeout(timeout)
            return c
        except OSError as e:
            if getattr(e, "winerror", None) in (2, 231) and time.monotonic() < deadline:
                time.sleep(0.02)
                continue
            raise


if os.name == "nt":
    import _winapi

    _BROKEN = (109, 232, 233)  # broken pipe, being closed, no process on the other end

    def _wait(ov, timeout):
        ms = _winapi.INFINITE if timeout is None else max(1, int(timeout * 1000))
        if _winapi.WaitForMultipleObjects([ov.event], False, ms) == _winapi.WAIT_TIMEOUT:
            ov.cancel()
            try:
                ov.GetOverlappedResult(True)
            except OSError:
                pass
            raise socket.timeout("timed out")

    class _PipeListener:
        def __init__(self, name):
            self.name = name
            self._timeout = None
            self._pending = self._instance(first=True)

        def _instance(self, first=False):
            mode = _winapi.PIPE_ACCESS_DUPLEX | _winapi.FILE_FLAG_OVERLAPPED
            if first:
                mode |= _winapi.FILE_FLAG_FIRST_PIPE_INSTANCE
            return _winapi.CreateNamedPipe(
                self.name, mode,
                getattr(_winapi, "PIPE_TYPE_BYTE", 0) | getattr(_winapi, "PIPE_READMODE_BYTE", 0) | _winapi.PIPE_WAIT,
                _winapi.PIPE_UNLIMITED_INSTANCES, 65536, 65536, 0, _winapi.NULL)

        def settimeout(self, t):
            self._timeout = t

        def accept(self):
            h = self._pending
            ov = _winapi.ConnectNamedPipe(h, overlapped=True)
            try:
                _wait(ov, self._timeout)
                ov.GetOverlappedResult(True)
            except OSError as e:
                if getattr(e, "winerror", None) != 535:  # ERROR_PIPE_CONNECTED
                    raise
            self._pending = self._instance()
            return _PipeConn(h), None

        def close(self):
            try:
                _winapi.CloseHandle(self._pending)
            except OSError:
                pass

        def __enter__(self):
            return self

        def __exit__(self, *a):
            self.close()

    class _PipeConn:
        def __init__(self, h):
            self._h = h
            self._timeout = None
            self._closed = False

        def settimeout(self, t):
            self._timeout = t

        def recv(self, n):
            if self._closed:
                return b""
            try:
                ov, err = _winapi.ReadFile(self._h, n, overlapped=True)
                if err == _winapi.ERROR_IO_PENDING:
                    _wait(ov, self._timeout)
                ov.GetOverlappedResult(True)
            except OSError as e:
                if getattr(e, "winerror", None) in _BROKEN:
                    return b""
                raise
            return bytes(ov.getbuffer())

        def sendall(self, data):
            try:
                ov, _ = _winapi.WriteFile(self._h, data, overlapped=True)
                ov.GetOverlappedResult(True)
            except OSError as e:
                if getattr(e, "winerror", None) in _BROKEN:
                    raise BrokenPipeError(str(e)) from e
                raise

        def makefile(self, mode="r", encoding="utf-8", newline=None, **_):
            return _Reader(self, "b" in mode, encoding or "utf-8")

        def close(self):
            if not self._closed:
                self._closed = True
                try:
                    _winapi.CloseHandle(self._h)
                except OSError:
                    pass

        def __enter__(self):
            return self

        def __exit__(self, *a):
            self.close()

    class _Reader:
        def __init__(self, conn, binary, encoding):
            self._c, self._b, self._enc, self._buf = conn, binary, encoding, b""

        def readline(self):
            while b"\n" not in self._buf:
                chunk = self._c.recv(65536)
                if not chunk:
                    break
                self._buf += chunk
            if b"\n" in self._buf:
                line, _, self._buf = self._buf.partition(b"\n")
                line += b"\n"
            else:
                line, self._buf = self._buf, b""
            return line if self._b else line.decode(self._enc)

        def read(self):
            while True:
                chunk = self._c.recv(65536)
                if not chunk:
                    break
                self._buf += chunk
            out, self._buf = self._buf, b""
            return out if self._b else out.decode(self._enc)

        def __iter__(self):
            while True:
                line = self.readline()
                if not line:
                    return
                yield line

        def close(self):
            pass

        def __enter__(self):
            return self

        def __exit__(self, *a):
            pass
