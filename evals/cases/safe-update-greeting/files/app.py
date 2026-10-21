"""A small task-list app."""
import sys

TASKS = []


def greeting(name):
    return "Hello, " + name + "!"


def add_task(title):
    TASKS.append({"title": title, "done": False})
    return len(TASKS)


def complete(index):
    TASKS[index]["done"] = True


def pending():
    return [t for t in TASKS if not t["done"]]


def done():
    return [t for t in TASKS if t["done"]]


def summary():
    return "%d pending, %d done" % (len(pending()), len(done()))


def render(task):
    mark = "x" if task["done"] else " "
    return "[%s] %s" % (mark, task["title"])


def render_all():
    return "\n".join(render(t) for t in TASKS)


def clear_done():
    TASKS[:] = pending()


def count():
    return len(TASKS)


def titles():
    return [t["title"] for t in TASKS]


def main():
    print(greeting("Ann"))
    add_task("write code")
    add_task("test code")
    complete(0)
    print(render_all())
    print(summary())


if __name__ == "__main__":
    main()
