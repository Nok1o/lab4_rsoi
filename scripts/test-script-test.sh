#!/usr/bin/env bash

# Exercise the cloud test orchestrator without contacting Kubernetes or an API.
set -euo pipefail
script_dir="$(cd "$(dirname "$0")" && pwd)"
exec python3 - "$script_dir" <<'PY'
import json
import os
from pathlib import Path
import re
import subprocess
import sys
import tempfile
import unittest

SCRIPTS = Path(sys.argv.pop(1)).resolve()
ROOT = SCRIPTS.parent

MOCK = r'''#!/usr/bin/env python3
import json
import os
from pathlib import Path
import re
import sys
from urllib.parse import urlparse

root = Path(os.environ["MOCK_ROOT"])
state_path = root / "state.json"
state = json.loads(state_path.read_text())
command = Path(sys.argv[0]).name
args = sys.argv[1:]

def event(kind, **fields):
    with (root / "events.jsonl").open("a") as output:
        output.write(json.dumps(dict(kind=kind, **fields)) + "\n")

def save():
    state_path.write_text(json.dumps(state))

def option(name, default=None):
    for index, value in enumerate(args):
        if value == name and index + 1 < len(args):
            return args[index + 1]
        if value.startswith(name + "="):
            return value.split("=", 1)[1]
    return default

event("command", command=command, args=args)
if command == "sleep":
    sys.exit(0)
if command == "kubectl":
    if option("-n", option("--namespace")) != "library-system":
        sys.exit("kubectl did not explicitly select the test namespace")
    if "scale" in args:
        state["replicas"] = int(option("--replicas"))
        event("scale", replicas=state["replicas"])
        save()
    elif "get" in args:
        resource = args[args.index("get") + 1]
        if resource in ("deployment", "deployments", "deploy"):
            if any("jsonpath" in arg for arg in args):
                print(state["replicas"], end="")
            else:
                print(json.dumps({"spec": {"replicas": state["replicas"]}}))
        elif resource in ("pod", "pods"):
            items = [] if state["replicas"] == 0 else [
                {"metadata": {"name": "rating-mock"},
                 "status": {"phase": "Running",
                            "conditions": [{"type": "Ready", "status": "True"}]}}]
            print(json.dumps({"items": items}))
        elif resource.startswith(("endpointslice", "endpoint")):
            endpoints = [] if state["replicas"] == 0 else [
                {"addresses": ["10.0.0.1"],
                 "conditions": {"ready": True, "serving": True}}]
            print(json.dumps({"items": [{"endpoints": endpoints,
                                        "subsets": ([{"addresses": [{"ip": "10.0.0.1"}]}]
                                                    if endpoints else [])}]}))
        else:
            sys.exit("unexpected kubectl get: " + resource)
    elif not any(verb in args for verb in ("rollout", "wait", "auth")):
        sys.exit("unexpected kubectl command")
elif command == "newman":
    folder = option("--folder")
    event("folder", folder=folder)
    environment = Path(option("--environment"))
    data = json.loads(environment.read_text())
    values = {item["key"]: item["value"] for item in data["values"]}
    if values["baseUrl"] != "https://cloud.example.invalid":
        sys.exit("Newman did not receive the cloud base URL")
    collection_path = Path(args[1])
    collection = json.loads(collection_path.read_text())
    group = next(item for item in collection["item"]
                 if item["name"] == ("success" if folder == "success" else "failover"))
    prerequest = next(item for item in group["event"] if item["listen"] == "prerequest")
    match = re.search(r'pm\.environment\.set\("username",\s*("CI-[^"]+")\)',
                      prerequest["script"]["exec"][-1])
    if not match:
        sys.exit("Temporary collection did not override the fixed test username")
    state["username"] = json.loads(match.group(1))
    # The temporary copy may change only the added username assignment.
    original = json.loads(Path(os.environ["SOURCE_COLLECTION"]).read_text())
    for item in collection["item"]:
        if item["name"] in ("success", "failover"):
            event_script = next(event for event in item["event"]
                                if event["listen"] == "prerequest")
            event_script["script"]["exec"].pop()
    if collection != original or collection_path == Path(os.environ["SOURCE_COLLECTION"]):
        sys.exit("Original collection assertions must remain unchanged")
    state["rental"] = folder == "step2" or (
        folder == "success" and os.environ.get("FAIL_FOLDER") == folder)
    if folder in ("success", "step2"):
        values["reservationUid"] = "11111111-1111-4111-8111-111111111111"
    values["stars"] = 75
    data["values"] = [{"key": key, "value": value, "enabled": True}
                      for key, value in values.items()]
    exported = Path(option("--export-environment"))
    exported.write_text(json.dumps(data))
    save()
    sys.exit(7 if os.environ.get("FAIL_FOLDER") == folder else 0)
elif command == "curl":
    url = next((value for value in args if value.startswith(("https://", "http://"))), "")
    path = urlparse(url).path
    method = option("--request", option("-X", "GET"))
    if path.endswith(("/reservations", "/return")):
        if option("--header") != "X-User-Name: " + state.get("username", "ci-mock"):
            sys.exit("Cleanup must query and return only the isolated test user's rentals")
    if path.endswith("/return"):
        event("return", path=path, method=method)
        if os.environ.get("FAIL_CLEANUP") == "1":
            sys.exit(22)
        state["rental"] = False
        save()
        body, status = "", "204"
    elif path.endswith("/reservations"):
        if os.environ.get("FAIL_CLEANUP_INSPECTION") == "1":
            sys.exit(22)
        username = state.get("username", "ci-mock")
        body = json.dumps([{
            "reservationUid": "11111111-1111-4111-8111-111111111111",
            "username": username, "status": "RENTED", "tillDate": "2021-10-11",
            "book": {"bookUid": "f7cdc58f-2caf-4b15-9727-f89dcc629b27"},
            "library": {"libraryUid": "83575e12-7ce0-48ee-9931-51919ff3c9ee"}
        }] if state.get("rental") else [])
        status = "200"
    elif path.endswith("/books"):
        body = json.dumps({"page": 1, "pageSize": 1, "totalElements": 1, "items": [{
            "bookUid": "f7cdc58f-2caf-4b15-9727-f89dcc629b27",
            "name": "Краткий курс C++ в 7 томах", "condition": "EXCELLENT",
            "availableCount": 1 if os.environ.get("UNAVAILABLE_BOOK") != "1" else 0
        }]})
        status = "200"
    elif path.endswith("/rating"):
        body, status = '{"stars":75}', "200"
    elif path.endswith("/health"):
        body, status = "{}", "200"
    else:
        sys.exit("unexpected curl URL: " + url)
    destination = option("--output", option("-o"))
    if destination:
        Path(destination).write_text(body)
    else:
        print(body, end="")
    write_out = option("--write-out", option("-w"))
    if write_out:
        print(write_out.replace("%{http_code}", status), end="")
else:
    sys.exit("unexpected mock command: " + command)
'''


class CloudScenarioTests(unittest.TestCase):
    def run_scenario(self, **overrides):
        with tempfile.TemporaryDirectory(prefix="lab4-scenario-test-") as directory:
            root = Path(directory)
            binary_dir = root / "bin"
            binary_dir.mkdir()
            for command in ("kubectl", "curl", "newman", "sleep"):
                executable = binary_dir / command
                executable.write_text(MOCK)
                executable.chmod(0o755)
            (root / "state.json").write_text(json.dumps({"replicas": 2, "rental": False}))
            fixtures = [ROOT / "v4/postman/environment.json", ROOT / "v4/postman/collection.json"]
            before = {path: path.read_bytes() for path in fixtures}
            environment = os.environ.copy()
            environment.update(
                PATH=str(binary_dir) + os.pathsep + environment["PATH"],
                MOCK_ROOT=str(root),
                BASE_URL="https://cloud.example.invalid",
                SOURCE_COLLECTION=str(ROOT / "v4/postman/collection.json"),
                TEST_REPORT_DIR=str(root / "reports"),
            )
            environment.update(overrides)
            result = subprocess.run(
                ["bash", str(SCRIPTS / "test-script.sh"), "v4", "rating-service", "library-system"],
                cwd=ROOT, env=environment, text=True, stdout=subprocess.PIPE,
                stderr=subprocess.STDOUT, timeout=30,
            )
            events_path = root / "events.jsonl"
            events = [json.loads(line) for line in events_path.read_text().splitlines()]
            state = json.loads((root / "state.json").read_text())
            for path, original in before.items():
                self.assertEqual(original, path.read_bytes(), "source fixture was modified: " + str(path))
            return result, events, state

    def test_success_runs_every_folder_and_restores_replicas(self):
        result, events, state = self.run_scenario()
        self.assertEqual(0, result.returncode, result.stdout)
        folders = [event["folder"] for event in events if event["kind"] == "folder"]
        self.assertEqual(["success", "step1", "step2", "step3", "step4"], folders)
        transitions = [event["replicas"] for event in events if event["kind"] == "scale"]
        self.assertEqual([0, 2, 0, 2, 2], transitions)
        sequence = [(event["kind"], event.get("folder", event.get("replicas")))
                    for event in events if event["kind"] in ("folder", "scale")]
        self.assertEqual([
            ("folder", "success"), ("scale", 0), ("folder", "step1"),
            ("scale", 2), ("folder", "step2"), ("scale", 0),
            ("folder", "step3"), ("scale", 2), ("folder", "step4"),
            ("scale", 2),
        ], sequence)
        self.assertEqual(2, state["replicas"])

    def test_failure_restores_replicas_and_returns_own_rental(self):
        result, events, state = self.run_scenario(FAIL_FOLDER="step2")
        self.assertEqual(7, result.returncode, result.stdout)
        self.assertEqual(["success", "step1", "step2"],
                         [event["folder"] for event in events if event["kind"] == "folder"])
        self.assertEqual(2, state["replicas"])
        returned = [event for event in events if event["kind"] == "return"]
        self.assertEqual(1, len(returned), result.stdout)
        self.assertIn("11111111-1111-4111-8111-111111111111", returned[0]["path"])
        self.assertEqual("POST", returned[0]["method"])
        self.assertFalse(state["rental"])

    def test_failure_while_rating_is_down_restores_replicas(self):
        result, events, state = self.run_scenario(FAIL_FOLDER="step1")
        self.assertNotEqual(0, result.returncode, result.stdout)
        self.assertEqual(2, state["replicas"])
        self.assertEqual(2, [event["replicas"] for event in events if event["kind"] == "scale"][-1])

    def test_cleanup_error_does_not_hide_test_failure(self):
        result, events, state = self.run_scenario(FAIL_FOLDER="step2", FAIL_CLEANUP="1")
        self.assertEqual(7, result.returncode, result.stdout)
        self.assertEqual(2, state["replicas"])
        self.assertTrue(any(event["kind"] == "return" for event in events), result.stdout)
        self.assertTrue(state["rental"])

    def test_cleanup_error_fails_an_otherwise_successful_suite(self):
        result, events, state = self.run_scenario(FAIL_CLEANUP_INSPECTION="1")
        self.assertNotEqual(0, result.returncode, result.stdout)
        self.assertEqual(["success", "step1", "step2", "step3", "step4"],
                         [event["folder"] for event in events if event["kind"] == "folder"])
        self.assertEqual(2, state["replicas"])
        self.assertIn("Recovery was incomplete", result.stdout)

    def test_unavailable_book_stops_before_newman_without_changing_other_users(self):
        result, events, state = self.run_scenario(UNAVAILABLE_BOOK="1")
        self.assertNotEqual(0, result.returncode, result.stdout)
        self.assertFalse(any(event["kind"] in ("folder", "return") for event in events))
        self.assertEqual(2, state["replicas"])


class KubernetesConfigurationTests(unittest.TestCase):
    token = "offline-dummy-iam-token"
    ca = "ZHVtbXktY2EtZm9yLW9mZmxpbmUtdGVzdHM="
    endpoint = "https://cluster.example.invalid"

    def setUp(self):
        self.directory = tempfile.TemporaryDirectory(prefix="lab4-kubeconfig-test-")
        self.addCleanup(self.directory.cleanup)
        self.root = Path(self.directory.name)
        self.output = self.root / "ci.kubeconfig"
        self.environment = os.environ.copy()
        self.environment.update(
            KUBECONFIG=str(self.output),
            YC_IAM_TOKEN=self.token,
            KUBE_NAMESPACE="library-system",
            KUBE_CONTEXT="offline-test",
        )

    def configure(self, source=None, **overrides):
        if source is None:
            source = {"server": self.endpoint, "certificate-authority-data": self.ca}
        environment = dict(self.environment, KUBE_CLUSTER_JSON=json.dumps(source), **overrides)
        return subprocess.run(
            ["bash", str(ROOT / "backend/deploy/configure-ci-kubeconfig.sh")],
            cwd=self.root, env=environment, text=True, stdout=subprocess.PIPE,
            stderr=subprocess.PIPE, timeout=10,
        )

    def export(self, cluster):
        binary_dir = self.root / "bin"
        binary_dir.mkdir(exist_ok=True)
        kubectl = binary_dir / "kubectl"
        kubectl.write_text(
            "#!/usr/bin/env python3\n"
            "import os, sys\n"
            "assert 'config' in sys.argv and 'view' in sys.argv\n"
            "assert '--minify' in sys.argv and '--flatten' in sys.argv and '--raw' in sys.argv\n"
            "sys.stdout.write(os.environ['MOCK_KUBECONFIG'])\n"
        )
        kubectl.chmod(0o755)
        environment = dict(self.environment)
        environment.update(
            PATH=str(binary_dir) + os.pathsep + environment["PATH"],
            MOCK_KUBECONFIG=json.dumps({
                "clusters": [{"name": "offline-test", "cluster": cluster}],
                "users": [{"name": "administrator", "user": {
                    "token": self.token,
                    "client-key-data": "must-not-be-exported",
                    "exec": {"command": "must-not-be-executed"},
                }}],
            }),
        )
        return subprocess.run(
            ["bash", str(ROOT / "backend/deploy/export-cluster-config.sh")],
            cwd=self.root, env=environment, text=True, stdout=subprocess.PIPE,
            stderr=subprocess.PIPE, timeout=10,
        )

    def test_temporary_kubeconfig_is_private_and_token_is_not_printed(self):
        result = self.configure()
        self.assertEqual(0, result.returncode, result.stderr)
        config = json.loads(self.output.read_text())
        self.assertEqual(0o600, self.output.stat().st_mode & 0o777)
        self.assertEqual(self.token, config["users"][0]["user"]["token"])
        self.assertEqual("library-system", config["contexts"][0]["context"]["namespace"])
        self.assertEqual(self.endpoint, config["clusters"][0]["cluster"]["server"])
        self.assertNotIn(self.token, result.stdout + result.stderr)

    def test_export_contains_only_endpoint_and_embedded_ca(self):
        result = self.export({
            "server": self.endpoint, "certificate-authority-data": self.ca,
            "proxy-url": "https://untrusted-proxy.example.invalid",
            "tls-server-name": "untrusted-name",
        })
        self.assertEqual(0, result.returncode, result.stderr)
        self.assertEqual({
            "server": self.endpoint, "certificate-authority-data": self.ca,
        }, json.loads(result.stdout))
        self.assertNotIn(self.token, result.stdout + result.stderr)
        self.assertNotIn("must-not-be", result.stdout + result.stderr)

    def test_export_rejects_insecure_or_missing_ca_configuration(self):
        for cluster in (
            {"server": "http://cluster.example.invalid", "certificate-authority-data": self.ca},
            {"server": self.endpoint, "certificate-authority-data": self.ca,
             "insecure-skip-tls-verify": True},
            {"server": self.endpoint},
        ):
            with self.subTest(cluster=cluster):
                result = self.export(cluster)
                self.assertNotEqual(0, result.returncode)
                self.assertEqual("", result.stdout)

    def test_configure_rejects_insecure_input_before_creating_output(self):
        for source in (
            {"server": "http://cluster.example.invalid", "certificate-authority-data": self.ca},
            {"server": self.endpoint, "certificate-authority-data": self.ca,
             "insecure-skip-tls-verify": True},
            {"server": self.endpoint, "certificate-authority-data": ""},
        ):
            with self.subTest(source=source):
                result = self.configure(source)
                self.assertNotEqual(0, result.returncode)
                self.assertFalse(self.output.exists())
                self.assertNotIn(self.token, result.stdout + result.stderr)

    def test_configure_drops_exec_proxy_and_supplied_user_credentials(self):
        marker = self.root / "exec-was-run"
        result = self.configure({
            "server": self.endpoint, "certificate-authority-data": self.ca,
            "proxy-url": "https://untrusted-proxy.example.invalid",
            "exec": {"command": "touch", "args": [str(marker)]},
            "auth-provider": {"name": "untrusted"},
            "users": [{"name": "untrusted", "user": {"token": "untrusted-token"}}],
        })
        self.assertEqual(0, result.returncode, result.stderr)
        config = json.loads(self.output.read_text())
        self.assertEqual({
            "server": self.endpoint, "certificate-authority-data": self.ca,
        }, config["clusters"][0]["cluster"])
        self.assertEqual({"token": self.token}, config["users"][0]["user"])
        self.assertFalse(marker.exists())
        self.assertNotIn("untrusted", self.output.read_text())

    def test_existing_output_is_never_overwritten(self):
        original = "existing credentials must survive\n"
        self.output.write_text(original)
        result = self.configure()
        self.assertNotEqual(0, result.returncode)
        self.assertEqual(original, self.output.read_text())
        self.assertNotIn(self.token, result.stdout + result.stderr)


class ClassroomEnvironmentTests(unittest.TestCase):
    def setUp(self):
        self.directory = tempfile.TemporaryDirectory(prefix="lab4-classroom-test-")
        self.addCleanup(self.directory.cleanup)
        self.root = Path(self.directory.name)
        (self.root / ".github/classroom").mkdir(parents=True)
        (self.root / "scripts").mkdir()
        (self.root / "scripts/test-script.sh").write_text(
            "#!/usr/bin/env bash\n"
            "exec python3 - \"$@\" <<'CAPTURE'\n"
            "import json, os, sys\n"
            "print(json.dumps({'variables': dict(os.environ), 'arguments': sys.argv[1:]}))\n"
            "CAPTURE\n"
        )
        configuration = json.loads((ROOT / ".github/classroom/autograding.json").read_text())
        self.command = configuration["tests"][0]["run"]
        # Match the environment whitelist used by education/autograding@v1,
        # intentionally dropping the workflow's other environment variables.
        self.child_environment = {"HOME": str(self.root), "PATH": os.environ["PATH"]}
        self.expected = {
            "BASE_URL": "https://cloud.example.invalid/path with spaces?q='\"&value=$HOME;$(touch OWNED)",
            "KUBECONFIG": str(self.root / "runner files" / "ci 'quoted'.config"),
            "TEST_REPORT_DIR": str(self.root / 'reports "quoted" and $(touch OWNED)') + "\nsecond line",
        }

    def preparation_script(self):
        lines = (ROOT / ".github/workflows/classroom.yml").read_text().splitlines()
        name = "      - name: Prepare environment for Classroom test process"
        self.assertIn(name, lines, "The real workflow must prepare the Classroom subprocess environment")
        start = lines.index(name) + 1
        finish = next((index for index in range(start, len(lines))
                       if lines[index].startswith("      - ")), len(lines))
        step = lines[start:finish]
        self.assertIn("        run: |", step)
        run = step[step.index("        run: |") + 1:]
        self.assertTrue(all(not line.strip() or line.startswith("          ") for line in run))
        return "\n".join(line[10:] for line in run) + "\n"

    def run_classroom(self):
        return subprocess.run(
            ["bash", "-c", self.command], cwd=self.root,
            env=self.child_environment, text=True, stdout=subprocess.PIPE,
            stderr=subprocess.PIPE, timeout=10,
        )

    def test_actual_workflow_transfers_only_required_values_to_sanitized_classroom(self):
        preparation_environment = dict(os.environ, **self.expected)
        preparation_environment.update(
            ARBITRARY_SECRET="dummy-secret-that-must-not-be-transferred",
            YC_IAM_TOKEN="dummy-iam-token-that-must-not-be-transferred",
            DOCKERHUB_TOKEN="dummy-docker-token-that-must-not-be-transferred",
        )
        preparation = subprocess.run(
            ["bash", "-e", "-c", self.preparation_script()], cwd=self.root,
            env=preparation_environment, text=True, stdout=subprocess.PIPE,
            stderr=subprocess.PIPE, timeout=10,
        )
        self.assertEqual(0, preparation.returncode, preparation.stderr)
        environment_file = self.root / ".github/classroom/cloud-test.env"
        self.assertEqual(0o600, environment_file.stat().st_mode & 0o777)
        exports = re.findall(r"^export ([A-Z_]+)=", environment_file.read_text(), re.MULTILINE)
        self.assertEqual(sorted(self.expected), sorted(exports))
        for name in ("ARBITRARY_SECRET", "YC_IAM_TOKEN", "DOCKERHUB_TOKEN"):
            self.assertNotIn(preparation_environment[name], environment_file.read_text())
            self.assertNotIn(preparation_environment[name], preparation.stdout + preparation.stderr)
        result = self.run_classroom()
        self.assertEqual(0, result.returncode, result.stderr)
        captured = json.loads(result.stdout)
        self.assertEqual(["v4", "rating-service", "library-system"], captured["arguments"])
        for name, value in self.expected.items():
            self.assertEqual(value, captured["variables"].get(name), name)
        # Bash/Python may add platform variables (e.g. LC_CTYPE or macOS
        # __CF_USER_TEXT_ENCODING); compare against a clean child baseline.
        baseline = subprocess.run(
            ["bash", "scripts/test-script.sh"], cwd=self.root,
            env=self.child_environment, text=True, stdout=subprocess.PIPE,
            stderr=subprocess.PIPE, timeout=10,
        )
        self.assertEqual(0, baseline.returncode, baseline.stderr)
        allowed = set(self.expected) | set(json.loads(baseline.stdout)["variables"])
        self.assertFalse(set(captured["variables"]) - allowed, captured["variables"])
        self.assertFalse((self.root / "OWNED").exists(), "Quoted values must never execute as shell code")

    def test_actual_autograding_command_fails_when_environment_file_is_missing(self):
        result = self.run_classroom()
        self.assertNotEqual(0, result.returncode)
        self.assertEqual("", result.stdout, "The test runner must not start without its prepared environment")


unittest.main(verbosity=2)
PY
