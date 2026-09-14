"""Re-export of fixtures/make_release_fixture.py's tree builder for the compat test."""
import importlib.util
import os

_path = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "fixtures", "make_release_fixture.py")
_spec = importlib.util.spec_from_file_location("make_release_fixture", _path)
_mod = importlib.util.module_from_spec(_spec)
_spec.loader.exec_module(_mod)

build_tree = _mod.build_tree
TREE = _mod.TREE
FIXED_MTIME = _mod.FIXED_MTIME
