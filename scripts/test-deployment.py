"""Deployment-generator and provider-contract tests. No public DNS is modified."""
import importlib.util
import io
import json
from pathlib import Path
import subprocess
import tempfile
import unittest
import urllib.error

ROOT = Path(__file__).resolve().parents[1]


def module(name, path):
    spec = importlib.util.spec_from_file_location(name, ROOT / path)
    result = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(result)
    return result


generator = module("docker_generate", "deploy/docker/generate.py")
cloud = module("cloud_generate", "deploy/cloud-init.py")
dns = module("cloudflare_dns", "deploy/cloudflare-dns.py")


class Response(io.BytesIO):
    pass


class Transport:
    def __init__(self, records):
        self.records = records
        self.requests = []

    def open(self, request, timeout):
        self.requests.append(request)
        assert request.full_url.startswith("https://api.cloudflare.com/client/v4/zones/")
        assert "private-token" not in request.full_url
        assert timeout == 15
        result = self.records if request.method == "GET" else {"id": "b" * 32}
        return Response(json.dumps({"success": True, "result": result}).encode())


class DeploymentTests(unittest.TestCase):
    def test_generated_keys_are_private_and_never_replaced(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            generator.generate(root, "VPN.Example.com")
            self.assertEqual((root / ".env").read_text(), "VELORAY_DOMAIN=vpn.example.com\n")
            self.assertEqual((root / "secrets").stat().st_mode & 0o777, 0o700)
            before = {item.name: item.read_bytes() for item in (root / "secrets").iterdir()}
            self.assertEqual(len(before), 5)
            self.assertEqual(len(set(before.values())), 5)
            with self.assertRaises(ValueError):
                generator.generate(root, "other.example.com")
            self.assertEqual(before, {item.name: item.read_bytes() for item in (root / "secrets").iterdir()})

    def test_hostname_and_shell_inputs_are_rejected(self):
        for value in ("example.com;id", "https://example.com", "example.com\nX=y", "foo..com", "127.0.0.1", "-host.example"):
            with tempfile.TemporaryDirectory() as directory, self.assertRaises(ValueError):
                generator.generate(Path(directory), value)
        for revision in ("main;id", "$(id)", "abc", "../other"):
            with self.assertRaises(ValueError):
                cloud.cloud_config("vpn.example.com", revision)

    def test_firstboot_has_ci_integrity_gate_and_valid_shell(self):
        data = cloud.cloud_config("vpn.example.com")
        script = data["write_files"][0]["content"]
        self.assertIn("vr_auto_ci_state", script)
        self.assertIn("verify-runtime.sh", script)
        self.assertIn("sha256sum --check", script)
        self.assertNotIn("PASSWORD=", script)
        self.assertNotIn("curl -k", script)
        subprocess.run(["bash", "-n"], input=script.encode(), check=True)

    def test_dns_create_and_update_use_exact_name_and_no_credentials_in_url(self):
        transport = Transport([])
        self.assertEqual(dns.upsert("a" * 32, "VPN.Example.com", "203.0.113.5", "private-token", opener=transport), "created")
        self.assertEqual(transport.requests[1].method, "POST")
        payload = json.loads(transport.requests[1].data)
        self.assertEqual(payload, {"type": "A", "name": "vpn.example.com", "content": "203.0.113.5", "ttl": 1, "proxied": False})
        transport = Transport([{"id": "b" * 32, "name": "vpn.example.com", "type": "AAAA"}])
        self.assertEqual(dns.upsert("a" * 32, "vpn.example.com", "2001:db8::1", "private-token", opener=transport), "updated")
        self.assertEqual(transport.requests[1].method, "PATCH")
        self.assertEqual(transport.requests[1].get_header("Authorization"), "Bearer private-token")

    def test_dns_ambiguity_and_redirects_fail_closed(self):
        for records in ([{}, {}], [{"id": "b" * 32, "name": "wrong.example", "type": "A"}]):
            transport = Transport(records)
            with self.assertRaises(RuntimeError):
                dns.upsert("a" * 32, "vpn.example.com", "203.0.113.5", "private-token", opener=transport)
            self.assertEqual(len(transport.requests), 1)
        self.assertIsNone(dns.NoRedirect().redirect_request(None, None, None, None, None, None))


if __name__ == "__main__":
    unittest.main()
