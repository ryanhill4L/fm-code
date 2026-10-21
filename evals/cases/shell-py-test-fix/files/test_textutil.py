import unittest
from textutil import word_count, shout, first_word


class T(unittest.TestCase):
    def test_count(self):
        self.assertEqual(word_count("a b c"), 3)

    def test_shout(self):
        self.assertEqual(shout("hi"), "HI!")

    def test_first(self):
        self.assertEqual(first_word("hello world"), "hello")


if __name__ == "__main__":
    unittest.main()
