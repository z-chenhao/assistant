"""Reusable Unix PTY drive for the real personal-agent entry point.

Owns only its child and PTY. Never writes an unredacted authentication URL to
evidence. Used by terminal acceptance and the Case Runner.
"""
import errno
import fcntl
import os
import pty
import re
import select
import subprocess
import termios
import time


class Terminal:
    def __init__(self, args, env, cwd):
        self.master, slave = pty.openpty()
        settings = termios.tcgetattr(slave)
        settings[3] &= ~termios.ECHO
        termios.tcsetattr(slave, termios.TCSANOW, settings)
        self.input_observer = os.dup(slave)
        self.original_flags = fcntl.fcntl(self.input_observer, fcntl.F_GETFL)
        try:
            self.process = subprocess.Popen(args, stdin=slave, stdout=slave,
                                            stderr=slave, env=env, cwd=cwd)
        finally:
            os.close(slave)
        self.output = b""
        self.position = 0

    def expect(self, marker, timeout=20):
        pattern = marker.encode()
        deadline = time.monotonic() + timeout
        while pattern not in self.output[self.position:]:
            remaining = deadline - time.monotonic()
            if remaining <= 0 or not select.select([self.master], [], [], remaining)[0]:
                raise RuntimeError(f"terminal timed out awaiting {marker!r}; {self.transcript()[-1200:]}")
            try:
                chunk = os.read(self.master, 65536)
            except OSError as error:
                if error.errno != errno.EIO:
                    raise
                chunk = b""
            if not chunk:
                raise RuntimeError(f"terminal exited awaiting {marker!r}; {self.transcript()[-1200:]}")
            self.output += chunk
        end = self.output.index(pattern, self.position) + len(pattern)
        value = self.output[self.position:end].decode(errors="replace")
        self.position = end
        return value

    def send(self, text):
        if "\n" in text or "\r" in text:
            raise ValueError("send exactly one terminal line")
        os.write(self.master, (text + "\n").encode())

    def transcript(self):
        text = self.output.decode(errors="replace").replace("\r\n", "\n")
        return re.sub(r"https://auth\.openai\.com/[^\s]+", "[private-login-url]", text)

    def stop(self, sig=None):
        if sig is None:
            self.send("/quit")
        else:
            self.process.send_signal(sig)
        code = self.process.wait(timeout=20)
        final_flags = fcntl.fcntl(self.input_observer, fcntl.F_GETFL)
        # Darwin also sets a status bit during ordinary PTY writes (a plain
        # Python input/print reproduces it). The adapter owns O_NONBLOCK only.
        if code == 0 and (final_flags & os.O_NONBLOCK) != (self.original_flags & os.O_NONBLOCK):
            raise RuntimeError(f"terminal changed the invoking shell's input flags: {self.original_flags:#x} -> {final_flags:#x}")
        return code

    def close(self):
        if self.process.poll() is None:
            self.process.kill()
        self.process.wait(timeout=20)
        os.close(self.master)
        os.close(self.input_observer)

    def __enter__(self):
        return self

    def __exit__(self, *_):
        self.close()
