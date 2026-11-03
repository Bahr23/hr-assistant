from html import escape


def question_rich_html(
    *,
    text: str,
    answer_type: str,
    question_number: int,
    question_total: int,
) -> str:
    answer_label = "текстовым" if answer_type == "text" else "голосовым"
    return f'''
<table>
    <tr>
        <th align="left">{escape(text)}</th>
    </tr>
    <tr>
        <th align="right"><sub>{question_number}/{question_total}</sub></th>
    </tr>
</table>
<footer>Ответьте {answer_label} сообщением.</footer>
'''
