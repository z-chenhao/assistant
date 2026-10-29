"""Reusable Unix PTY drive for the real collie entry point.

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

from fixture_pool import current


class Terminal:
    def __init__(self, args, env, cwd, echo=False):
        self.processes = current()
        self.master, slave = pty.openpty()
        settings = termios.tcgetattr(slave)
        if not echo:
            settings[3] &= ~termios.ECHO
        termios.tcsetattr(slave, termios.TCSANOW, settings)
        self.input_observer = os.dup(slave)
        self.original_flags = fcntl.fcntl(self.input_observer, fcntl.F_GETFL)
        self.original_settings = termios.tcgetattr(self.input_observer)
        os.set_blocking(self.master, False)
        try:
            start = self.processes.start if self.processes is not None else subprocess.Popen
            self.process = start(args, stdin=slave, stdout=slave,
                                            stderr=slave, env=env, cwd=cwd)
        except BaseException:
            os.close(self.master)
            os.close(self.input_observer)
            raise
        finally:
            os.close(slave)
        self.output = b""
        self.position = 0

    def expect(self, marker, timeout=20):
        pattern = marker.encode()
        deadline = time.monotonic() + timeout
        while pattern not in self.output[self.position:]:
            if self.processes is not None:
                self.processes.check()
            remaining = deadline - time.monotonic()
            if remaining <= 0:
                raise RuntimeError(f"terminal timed out awaiting {marker!r}; {self.transcript()[-1200:]}")
            # The retained slave observer prevents EOF after a scoped child
            # stops. Poll cancellation while preserving the absolute deadline.
            wait = min(remaining, .05) if self.processes is not None else remaining
            if not select.select([self.master], [], [], wait)[0]:
                continue
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

    def expect_all(self, markers, timeout=20):
        """Await independently ordered output from concurrent foreground work."""
        start, end = self.position, self.position
        values = []
        for marker in markers:
            self.position = start
            values.append(self.expect(marker, timeout))
            end = max(end, self.position)
        self.position = end
        return values

    def send(self, text):
        if "\n" in text or "\r" in text:
            raise ValueError("send exactly one terminal line")
        self.send_bytes((text + "\n").encode())

    def send_bytes(self, data):
        # Large pastes may be partially written. Drain concurrent terminal echo
        # while sending so neither PTY direction can deadlock the other.
        remaining = memoryview(data)
        deadline = time.monotonic() + 20
        while remaining:
            if self.processes is not None:
                self.processes.check()
            timeout = deadline - time.monotonic()
            if timeout <= 0:
                raise RuntimeError("terminal input write timed out")
            wait = min(timeout, .05) if self.processes is not None else timeout
            readable, writable, _ = select.select([self.master], [self.master], [], wait)
            if readable:
                self.output += os.read(self.master, 65536)
            if writable:
                try:
                    written = os.write(self.master, remaining[:4096])
                    remaining = remaining[written:]
                except BlockingIOError:
                    pass

    def transcript(self):
        text = self.output.decode(errors="replace").replace("\r\n", "\n")
        return re.sub(r"https://auth\.openai\.com/[^\s]+", "[private-login-url]", text)

    def stop(self, sig=None):
        if self.process.poll() is None:
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
        final_settings = termios.tcgetattr(self.input_observer)
        # Darwin sets PENDIN when canonical input is restored, including with
        # stdlib tcsetattr alone. It is a transient retype status, not a mode.
        final_settings[3] &= ~termios.PENDIN
        original_settings = self.original_settings.copy()
        original_settings[3] &= ~termios.PENDIN
        if code == 0 and final_settings != original_settings:
            raise RuntimeError(f"terminal did not restore its original termios settings: {original_settings!r} -> {final_settings!r}")
        return code

    def close(self):
        if self.process.poll() is None:
            self.process.kill()
        self.process.wait(timeout=20)
        os.close(self.master)
        os.close(self.input_observer)
        if self.processes is not None:
            self.processes.retire(self.process)

    def __enter__(self):
        return self

    def __exit__(self, *_):
        self.close()
