import unittest

from src.rich_messages import question_rich_html


class RichMessageTests(unittest.TestCase):
    def test_builds_text_question_and_escapes_content(self) -> None:
        html = question_rich_html(
            text='Опыт с Go < 3 лет & "почему"?',
            answer_type="text",
            question_number=1,
            question_total=2,
        )

        self.assertIn(
            '<th align="left">Опыт с Go &lt; 3 лет &amp; &quot;почему&quot;?</th>', html
        )
        self.assertIn('<th align="right"><sub>1/2</sub></th>', html)
        self.assertIn(
            "<footer>Ответьте текстовым сообщением.</footer>", html
        )

    def test_builds_voice_hint(self) -> None:
        html = question_rich_html(
            text="Расскажите о себе",
            answer_type="voice",
            question_number=2,
            question_total=3,
        )

        self.assertIn("<sub>2/3</sub>", html)
        self.assertIn(
            "<footer>Ответьте голосовым сообщением.</footer>", html
        )


if __name__ == "__main__":
    unittest.main()
