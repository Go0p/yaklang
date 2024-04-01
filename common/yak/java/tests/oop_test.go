package tests

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi/ssatest"
)

func TestJava_OOP_Var_member(t *testing.T) {

	t.Run("test simple member call", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`
class A {
		int a = 0;

}
class Main{
		public static void main(String[] args) {
			A a = new A();
			println(a.a);
		}
}
		`, []string{"0"}, t)
	})

	t.Run("side effect", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`
	 class A {
		private int a = 0; 
	
		public void setA(int par) { 
			this.a = par;
		}
	}
	class Main{
		public static void main(String[] args) {
			A a = new A();
			println(a.a);
			a.setA(1);
			println(a.a);
		}
	}
		`, []string{
			"0", "side-effect(Parameter-par, this.a)",
		}, t)
	})

	t.Run("free-value", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`
		class A {
			int a = 0; 
			public void getA() {
				return this.a;
			}
		}

		class Main{
		public static void main(String[] args) {
		A a = new A(); 
		println(a.getA());
		a.a=1;
		println(a.getA());
		}
}
		`, []string{
			"Function-A_getA(make(A),0)",
			"Function-A_getA(make(A),1)",
		}, t)
	})

	t.Run("just use method", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`
	public 	class A {
			int a = 0; 
			public void getA(){
			return this.a;
			}
			
			public void setA(int par){
			this.a=par;
			}
		}
	public class Main{
		public static void main(String[] args) {
		A a = new A(); 
		println(a.getA());
		a.setA(1);
		println(a.getA());
		}
}
		`, []string{
			"Function-A_getA(make(A),0)",
			"Function-A_getA(make(A),side-effect(Parameter-par, this.a))",
		}, t)
	})
}

func TestJava_Extend_Class(t *testing.T) {

	t.Run("test extend constant ", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`
		class A {
			int a = 0; 
		}
	public class B extends A{}
	public class C extends B{}
	public class Main{
		public static void main(String[] args) {
		C C = new C();
		println(C.a); // 0
}}
		`, []string{
			"0",
		}, t)
	})

	t.Run("free-value", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`
	public 	class Q {
			int a = 0; 
			public void getA() {
				return this.a;
			}
		}
		class A extends Q{}
		public class Main{
		public static void main(String[] args) {
			
		A a = new A(); 
		println(a.getA());
		a.a=1;
		println(a.getA());
		}
}
		`, []string{
			"Function-Q_getA(make(A),0)",
			"Function-Q_getA(make(A),1)",
		}, t)
	})

	t.Run("just use method", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`
		public class Q {
			int a = 0; 
			public void getA(){
			return this.a;
			}
			
			public void setA(int par){
			this.a=par;
			}
		}
		class A extends Q{}
		public class Main{
		public static void main(String[] args) {
		A a = new A(); 
		println(a.getA());
		a.setA(1);
		println(a.getA());
		}
}
		`, []string{
			"Function-Q_getA(make(A),0)",
			"Function-Q_getA(make(A),side-effect(Parameter-par, this.a))",
		}, t)
	})
}

func TestJava_Construct(t *testing.T) {
	t.Run("no construct", func(t *testing.T) {
		code := `
	public	class A {
			int num = 0;
			public int getNum() {
				super();
				return this.num;
			}
		}
public class Main{
		public static void main(String[] args) {
		A a = new A(); 
		println(a.getNum());
		}
}
		`
		ssatest.CheckPrintlnValue(code, []string{
			"Function-A_getNum(make(A),0)",
		}, t)
	})

	t.Run("normal construct", func(t *testing.T) {
		code := `
public class A {
	private int num1=0;
	private int num2=0;
	
	public A(int num1,int num2) {
		this.num1 = num1;
		this.num2 = num2;

	}
	public int getNum1() {
		return this.num1;
	}
	public int getNum2(){
	return this.num2;
}
}
public class Main{
		public static void main(String[] args) {
		A a = new A(1,2);
		println(a.getNum1());
		println(a.getNum2());
		}
}
`
		ssatest.CheckPrintlnValue(code, []string{
			"Function-A_getNum1(make(A),side-effect(Parameter-num1, this.num1))",
			"Function-A_getNum2(make(A),side-effect(Parameter-num2, this.num2))",
		}, t)
	})
}

func TestJava_OOP_Enum(t *testing.T) {
	t.Run("test simple top-level enum", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`
		public enum A {
			A,B,C;
		}
		public class Main{
			public static void main(String[] args) {
			A a = A.B;
			println(a);
			}
		}
		`, []string{
			"make(A)",
		}, t)
	})

	t.Run("test  top-level enum with constructor", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`
		public enum A {
			A(1,2),
			B(3,4),
			C(4,5);
			private final int num1;
			private final int num2;

			A(int par1,int par2){
				this.num1=par1;
				this.num2=par2;
			}

			public int getNum1(){
			return this.num1;
			}

			public int getNum2(){
			return this.num2;
			}
		}
		public class Main{
			public static void main(String[] args) {
			A a = A.B;
			println(a.getNum1());
			println(a.getNum2());
			}
		}
		`, []string{
			"Function-A_getNum1(make(A),side-effect(Parameter-par1, a.num1))",
			"Function-A_getNum2(make(A),side-effect(Parameter-par2, a.num2))",
		}, t)
	})

}

func TestJava_OOP_MemberClass(t *testing.T) {
	t.Run("test no-static inner class ", func(t *testing.T) {
		code := `
public class Outer {
    public  class Inner{
        int a = 1;
        public Inner(int par){
            this.a=par;
        }
        public int getA(){
            return this.a;
        }
    }
}

public class Main{
    public static void main(String[] args) {
        Outer outer = new Outer();
        Outer.Inner inner =outer.new Inner(5);
        println(inner);
		println(inner.getA());
    }
}`
		ssatest.CheckPrintlnValue(code, []string{
			"make(Outer.Inner)",
			"Function-Outer.Inner_getA(make(Outer.Inner),side-effect(Parameter-par, this.a))",
		}, t)
	})
}

func TestJava_OOP_Static_Member(t *testing.T) {
	t.Run("test static member 1", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`
class A {
		static int a = 0;

}
class Main{
		public static void main(String[] args) {
			A a = new A();
			println(a.a);
		}
}
		`, []string{"Undefined-a.a(valid)"}, t)
	})

	//TODO: static member need more code
	// 	t.Run("test static member 2", func(t *testing.T) {
	// 		ssatest.CheckPrintlnValue(`

	// class Main{
	// 		static int a = 0;
	// 		public static void main(String[] args) {
	// 			println(a);
	// 		}
	// }
	// 		`, []string{"0"}, t)
	// 	})

	t.Run("test static method  1", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`
class A {
		int a = 0;
		public static void Hello(){
        }

}
class Main{
		public static void main(String[] args) {
			A a = new A();
			println(a.Hello());
		}
}
		`, []string{"Undefined-a.Hello(valid)()"}, t)
	})

	t.Run("test static method  2", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`
class Main{
		public static void Hello(){
        }
		public static void main(String[] args) {
			println(Hello());
		}
}
		`, []string{"Function-Main_Hello()"}, t)
	})
}