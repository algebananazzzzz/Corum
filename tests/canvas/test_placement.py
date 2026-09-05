from corum.canvas import placement


FOLDERS = {"Labs": "labs", "Lecture Notes & Readings": "lectures"}


def test_place_maps_a_known_folder_to_the_vault_vocabulary():
    assert placement.place("Labs", "lab1.pdf", FOLDERS) == "labs/lab1.pdf"


def test_place_matches_the_longest_prefix_so_one_entry_covers_children():
    got = placement.place("Lecture Notes & Readings/Lecture 3", "L3.pdf", FOLDERS)
    assert got == "lectures/L3.pdf"


def test_place_mirrors_an_unmapped_folder_so_the_path_says_what_it_is():
    assert placement.place("CRAG Slides", "Intro.pdf", FOLDERS) == "CRAG Slides/Intro.pdf"


def test_place_falls_back_to_the_course_root_when_folders_is_unreadable():
    assert placement.place(None, "A1.pdf", FOLDERS) == "A1.pdf"


def test_place_prefers_the_longer_of_two_overlapping_mappings():
    folders = {"Lectures": "lectures", "Lectures/Archive": "references"}
    assert placement.place("Lectures/Archive", "old.pdf", folders) == "references/old.pdf"
