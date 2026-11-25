"""Smoke tests for the generated package."""

import example_project


def test_package_imports() -> None:
    """Verify that the package imports and has a docstring."""
    assert example_project.__doc__
