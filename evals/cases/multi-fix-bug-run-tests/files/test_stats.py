import unittest
from stats import mean, biggest


class T(unittest.TestCase):
    def test_mean(self):
        self.assertEqual(mean([2, 4, 6]), 4)

    def test_biggest(self):
        self.assertEqual(biggest([1, 9, 3]), 9)


if __name__ == "__main__":
    unittest.main()
