package tests

import (
	"testing"
)

func TestJava_Func_Params(t *testing.T) {
	t.Run("test  function params 1", func(t *testing.T) {
		CheckAllJavaPrintlnValue(`public class Main {
    public static void A(int a) {
        println(a);
    }

    public static void main(String[] args) {
        A(0); 
    }
}
`, []string{
			"Parameter-a",
		}, t)
	})
	t.Run("test  function params 2", func(t *testing.T) {
		CheckAllJavaPrintlnValue(`
public class Main {
    public static void A(int... a) {
        println(a);
    }

    public static void main(String[] args) {
        A(0); 
    }
}`, []string{
			"Parameter-a",
		}, t)
	})

	t.Run("test  function params 3", func(t *testing.T) {
		CheckAllJavaPrintlnValue(`
public class Main {
    public static void A(int a,boolen c,Dog d) {
        println(a);
		println(c);
		println(d);
    }

    public static void main(String[] args) {
        A(0); 
    }
}`, []string{
			"Parameter-a",
			"Parameter-c",
			"Parameter-d",
		}, t)
	})

	t.Run("test java not freeValue", func(t *testing.T) {
		CheckAllJavaPrintlnValue(`public class Main {
    public static int A() {
         println(a);
    }
public static void main(String[] args) {
	A();
    }
}
`, []string{"Undefined-a"}, t)
	})
}

func TestJava_Func_Closure(t *testing.T) {
	t.Run("test closure-variable in outside", func(t *testing.T) {
		CheckAllJavaPrintlnValue(`public class Main {
	public static int a = 1;
    public static int A(int a) {
         println(a);
    }
public static void main(String[] args) {
	A();
    }
}
`, []string{"Parameter-a"}, t)
	})

	t.Run("test closure-variable in inside", func(t *testing.T) {
		CheckAllJavaPrintlnValue(`public class Main {
	public static int a = 1;
    public static int A(int a) {
		 int a = 100;
         println(a);
    }
public static void main(String[] args) {
	A();
    }
}
`, []string{"100"}, t)
	})

}

func TestJava_FuncCall(t *testing.T) {
	t.Run("test  function use", func(t *testing.T) {
		CheckAllJavaPrintlnValue(`
public class Main {
    public static int A(int... a) {
        return a[0];
    }

    public static void main(String[] args) {
        println(A);
    }
}`, []string{
			"Function-A",
		}, t)
	})
}

func TestJava_Lambda(t *testing.T) {
	t.Run("test  function use", func(t *testing.T) {
		CheckAllJavaPrintlnValue(`
public class Main {
    public static int A(int... a) {
        return a[0];
    }

    public static void main(String[] args) {
        println(A);
    }
}`, []string{
			"Function-A",
		}, t)
	})
}
