import re

from main import create_app


def operations(document):
    for path, path_item in document["paths"].items():
        for method, operation in path_item.items():
            if method in {"get", "post", "put", "patch", "delete"}:
                yield path, method, operation


def test_business_api_uses_one_unversioned_root():
    paths = set(create_app().openapi()["paths"])
    business_paths = {path for path in paths if not path.startswith("/health/")}

    assert business_paths
    assert all(path.startswith("/api/") for path in business_paths)
    assert not any(re.match(r"^/api/v\d+(?:/|$)", path) for path in paths)
    assert "/api/monitors" in paths


def test_every_operation_has_stable_metadata_and_typed_success_response():
    seen = set()
    for path, method, operation in operations(create_app().openapi()):
        operation_id = operation.get("operationId")
        assert operation_id and operation_id not in seen, (method, path, operation_id)
        seen.add(operation_id)
        assert operation.get("tags"), (method, path)

        success = [code for code in operation["responses"] if code.startswith("2")]
        assert len(success) == 1, (method, path, success)
        response = operation["responses"][success[0]]
        if success[0] == "204":
            assert "content" not in response, (method, path)
        else:
            assert "schema" in response["content"]["application/json"], (method, path)


def test_all_business_operations_except_login_declare_session_security():
    for path, method, operation in operations(create_app().openapi()):
        if path.startswith("/health/") or (path == "/api/session" and method == "post"):
            assert "security" not in operation, (method, path)
        else:
            assert operation.get("security") == [{"OwnerSession": []}], (method, path)


def test_operations_only_advertise_errors_their_contract_can_return():
    document = create_app().openapi()

    assert set(document["paths"]["/health/live"]["get"]["responses"]) == {"200"}
    assert set(document["paths"]["/health/ready"]["get"]["responses"]) == {"200", "503"}
    assert set(document["paths"]["/api/session"]["post"]["responses"]) == {
        "200",
        "401",
        "403",
        "413",
        "422",
        "429",
        "500",
        "503",
    }
    assert set(document["paths"]["/api/monitors"]["get"]["responses"]) == {
        "200",
        "401",
        "413",
        "422",
        "500",
        "503",
    }
