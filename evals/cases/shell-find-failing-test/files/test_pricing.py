import unittest
from pricing import apply_discount, with_tax


class T(unittest.TestCase):
    def test_tax(self):
        self.assertEqual(with_tax(100), 110.0)

    def test_discount(self):
        self.assertEqual(apply_discount(200, 10), 180)


if __name__ == "__main__":
    unittest.main()
