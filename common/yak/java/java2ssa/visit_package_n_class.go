package java2ssa

import (
	"github.com/yaklang/yaklang/common/log"
	javaparser "github.com/yaklang/yaklang/common/yak/java/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func (y *builder) VisitTypeDeclaration(raw javaparser.ITypeDeclarationContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*javaparser.TypeDeclarationContext)
	if i == nil {
		return nil
	}

	var modifier []string
	for _, mod := range i.AllClassOrInterfaceModifier() {
		modifier = append(modifier, mod.GetText())
	}

	if ret := i.ClassDeclaration(); ret != nil {
		return y.VisitClassDeclaration(ret)
	} else if ret := i.EnumDeclaration(); ret != nil {
		return y.VisitEnumDeclaration(ret)
	} else if ret := i.InterfaceDeclaration(); ret != nil {
		return y.VisitInterfaceDeclaration(ret)
	} else if ret := i.AnnotationTypeDeclaration(); ret != nil {
		return y.VisitAnnotationTypeDeclaration(ret)
	} else if ret := i.RecordDeclaration(); ret != nil {
		return y.VisitRecordDeclaration(ret)
	}

	log.Errorf("visit type decl failed: %s", "unknown type")
	return nil
}

func (y *builder) VisitClassDeclaration(raw javaparser.IClassDeclarationContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*javaparser.ClassDeclarationContext)
	if i == nil {
		return nil
	}

	className := i.Identifier().GetText()
	log.Infof("building class: %v", className)

	// Generic Type
	if ret := i.TypeParameters(); ret != nil {
		log.Infof("class: %v 's (generic type) type is %v, ignore for ssa building", className, ret.GetText())
	}

	// Extend Type
	if extend := i.TypeType(); extend != nil {
		log.Infof("class: %v extend: %s", className, extend.GetText())
		y.VisitTypeType(extend)
	}

	haveImplements := false
	if i.IMPLEMENTS() != nil {
		haveImplements = true
		log.Infof("class: %v implemented %v is ignored", className, i.TypeList(0).GetText())
	}

	if i.PERMITS() != nil {
		idx := 1
		if !haveImplements {
			idx = 0
		}
		log.Infof("class: %v java17 permits: %v", className, i.TypeList(idx).GetText())
	}

	y.VisitClassBody(i.ClassBody())

	return nil
}

func (y *builder) VisitClassBody(raw javaparser.IClassBodyContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*javaparser.ClassBodyContext)
	if i == nil {
		return nil
	}

	ir := y
	allMembers := i.AllClassBodyDeclaration()
	pb := ir.EmitNewClassBluePrint(len(allMembers))
	for _, decl := range allMembers {
		instance := decl.(*javaparser.ClassBodyDeclarationContext)
		if ret := instance.Block(); ret != nil {
			isStatic := instance.STATIC() != nil
			log.Infof("handle class static code: %v", isStatic)
			y.VisitBlock(instance.Block())
		} else if ret := instance.MemberDeclaration(); ret != nil {
			y.VisitMemberDeclaration(pb, ret)
		}
	}

	return nil
}

func (y *builder) VisitFormalParameters(raw javaparser.IFormalParametersContext) {
	if y == nil || raw == nil {
		return
	}

	i, _ := raw.(*javaparser.FormalParametersContext)
	if i == nil {
		return
	}

	if i.ReceiverParameter() != nil && i.COMMA() == nil {
		y.VisitReceiverParameter(i.ReceiverParameter())
	} else if i.ReceiverParameter() != nil && i.COMMA() != nil {
		y.VisitReceiverParameter(i.ReceiverParameter())
		y.VisitFormalParameterList(i.FormalParameterList())
	} else if i.FormalParameterList() != nil && i.COMMA() == nil {
		y.VisitFormalParameterList(i.FormalParameterList())
	}

}

func (y *builder) VisitMemberDeclaration(klass *ssa.ClassBluePrint, raw javaparser.IMemberDeclarationContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*javaparser.MemberDeclarationContext)
	if i == nil {
		return nil
	}

	if ret := i.RecordDeclaration(); ret != nil {
		log.Infof("todo: java17: %v", ret.GetText())
	} else if ret := i.MethodDeclaration(); ret != nil {
		y.VisitMethodDeclaration(ret)
	} else if ret := i.GenericMethodDeclaration(); ret != nil {
	} else if ret := i.FieldDeclaration(); ret != nil {
		// 声明成员变量
		field := ret.(*javaparser.FieldDeclarationContext)
		variables := field.VariableDeclarators().(*javaparser.VariableDeclaratorsContext).AllVariableDeclarator()
		for _, variable := range variables {
			y.CreateLocalVariable(variable.GetText())
			log.Infof("create member declaration%v", variable.GetText())
		}

	} else if ret := i.ConstructorDeclaration(); ret != nil {

	} else if ret := i.GenericConstructorDeclaration(); ret != nil {

	} else if ret := i.InterfaceDeclaration(); ret != nil {

	} else if ret := i.AnnotationTypeDeclaration(); ret != nil {

	} else if ret := i.ClassDeclaration(); ret != nil {

	} else if ret := i.EnumDeclaration(); ret != nil {

	} else {
		log.Errorf("no member declaration found: %v", i.GetText())
		return nil
	}

	return nil
}

func (y *builder) VisitTypeType(raw javaparser.ITypeTypeContext) ssa.Type {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*javaparser.TypeTypeContext)
	if i == nil {
		return nil
	}
	// todo annotation
	var t ssa.Type
	if ret := i.ClassOrInterfaceType(); ret != nil {
		t = y.VisitClassOrInterfaceType(ret)
	} else {
		t = y.VisitPrimitiveType(i.PrimitiveType())
	}

	return t
}

func (y *builder) VisitClassOrInterfaceType(raw javaparser.IClassOrInterfaceTypeContext) ssa.Type {
	if y == nil || raw == nil {
		return nil
	}
	// todo 类和接口的类型声明
	i, _ := raw.(*javaparser.ClassOrInterfaceTypeContext)
	if i == nil {
		return nil
	}

	return nil
}

func (y *builder) VisitPrimitiveType(raw javaparser.IPrimitiveTypeContext) ssa.Type {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*javaparser.PrimitiveTypeContext)
	if i == nil {
		return nil
	}
	switch i.GetText() {
	case "boolean":
		return ssa.GetBooleanType()
	case "char", "short", "int", "long", "float", "double":
		return ssa.GetNumberType()
	case "byte":
		return ssa.GetBytesType()
	default:
		return ssa.GetAnyType()
	}
}

func (y *builder) VisitEnumDeclaration(raw javaparser.IEnumDeclarationContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*javaparser.EnumDeclarationContext)
	if i == nil {
		return nil
	}

	return nil
}

func (y *builder) VisitInterfaceDeclaration(raw javaparser.IInterfaceDeclarationContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*javaparser.InterfaceDeclarationContext)
	if i == nil {
		return nil
	}

	return nil
}

func (y *builder) VisitAnnotationTypeDeclaration(raw javaparser.IAnnotationTypeDeclarationContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*javaparser.AnnotationTypeDeclarationContext)
	if i == nil {
		return nil
	}

	return nil
}

func (y *builder) VisitRecordDeclaration(raw javaparser.IRecordDeclarationContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*javaparser.RecordDeclarationContext)
	if i == nil {
		return nil
	}

	return nil
}

func (y *builder) VisitMethodDeclaration(raw javaparser.IMethodDeclarationContext) {
	if y == nil || raw == nil {
		return
	}
	i, _ := raw.(*javaparser.MethodDeclarationContext)
	if i == nil {
		return
	}

	funcName := i.Identifier().GetText()

	y.SetMarkedFunction(funcName)
	newFunction := y.NewFunc(funcName)
	var ir *ssa.FunctionBuilder
	ir = y.PushFunction(newFunction)
	{
		y.VisitFormalParameters(i.FormalParameters())
		y.VisitMethodBody(i.MethodBody())
		ir.SetType(y.VisitTypeTypeOrVoid(i.TypeTypeOrVoid()))
		ir.Finish()
	}
	ir = ir.PopFunction()
	variable := ir.CreateVariable(funcName)
	ir.AssignVariable(variable, newFunction)
	if i.THROWS() != nil {
		if qualifiedNameList := i.QualifiedNameList(); qualifiedNameList != nil {
			y.VisitQualifiedNameList(qualifiedNameList)
		}

	}
	return
}

func (y *builder) VisitMethodBody(raw javaparser.IMethodBodyContext) {
	if y == nil || raw == nil {
		return
	}
	i, _ := raw.(*javaparser.MethodBodyContext)
	if i == nil {
		return
	}

	y.VisitBlock(i.Block())
}

func (y *builder) VisitTypeTypeOrVoid(raw javaparser.ITypeTypeOrVoidContext) ssa.Type {
	if y == nil || raw == nil {
		return nil
	}
	i, _ := raw.(*javaparser.TypeTypeOrVoidContext)
	if i == nil {
		return nil
	}
	if ret := i.TypeType(); ret != nil {
		return y.VisitTypeType(ret)
	} else {
		return ssa.GetAnyType()
	}

}

func (y *builder) VisitFormalParameterList(raw javaparser.IFormalParameterListContext) {
	if y == nil || raw == nil {
		return
	}

	i, _ := raw.(*javaparser.FormalParameterListContext)
	if i == nil {
		return
	}

	if allFormalParam := i.AllFormalParameter(); allFormalParam != nil {
		for _, param := range allFormalParam {
			y.VisitFormalParameter(param)
		}
		if lastFormalParam := i.LastFormalParameter(); lastFormalParam != nil {
			y.VisitLastFormalParameter(lastFormalParam)
		}
	} else {
		if lastFormalParam := i.LastFormalParameter(); lastFormalParam != nil {
			y.VisitLastFormalParameter(lastFormalParam)
		}
	}

}

func (y *builder) VisitReceiverParameter(raw javaparser.IReceiverParameterContext) {
	if y == nil || raw == nil {
		return
	}
	i, _ := raw.(*javaparser.ReceiverParameterContext)
	if i == nil {
		return
	}

	typeType := y.VisitTypeType(i.TypeType())
	_ = typeType
	// todo 接口的形参
}

func (y *builder) VisitFormalParameter(raw javaparser.IFormalParameterContext) {
	if y == nil || raw == nil {
		return
	}

	i, _ := raw.(*javaparser.FormalParameterContext)
	if i == nil {
		return
	}
	for _, modifier := range i.AllVariableModifier() {
		y.VisitVariableModifier(modifier)
	}
	typeType := y.VisitTypeType(i.TypeType())
	formalParams := y.VisitVariableDeclaratorId(i.VariableDeclaratorId())
	param := y.NewParam(formalParams)
	if typeType != nil {
		param.SetType(typeType)
	}

}

func (y *builder) VisitVariableDeclaratorId(raw javaparser.IVariableDeclaratorIdContext) string {
	if y == nil || raw == nil {
		return ""
	}
	i, _ := raw.(*javaparser.VariableDeclaratorIdContext)
	if i == nil {
		return ""
	}
	text := i.Identifier().GetText()
	if text == "" {
		return ""
	}
	y.CreateVariable(text)
	return text
}

func (y *builder) VisitLastFormalParameter(raw javaparser.ILastFormalParameterContext) {
	if y == nil || raw == nil {
		return
	}

	i, _ := raw.(*javaparser.LastFormalParameterContext)
	if i == nil {
		return
	}

	for _, modifier := range i.AllVariableModifier() {
		y.VisitVariableModifier(modifier)
	}

	for _, annotation := range i.AllAnnotation() {
		//todo annotation
		_ = annotation
		//y.VisitAnnotation(annotation)
	}
	formalParams := y.VisitVariableDeclaratorId(i.VariableDeclaratorId())
	typeType := y.VisitTypeType(i.TypeType())
	isVariadic := i.ELLIPSIS()
	_ = isVariadic
	param := y.NewParam(formalParams)
	if typeType != nil {
		param.SetType(typeType)
	}
}

func (y *builder) VisitVariableModifier(raw javaparser.IVariableModifierContext) {
	if y == nil || raw == nil {
		return
	}

	i, _ := raw.(*javaparser.VariableModifierContext)
	if i == nil {
		return
	}
}

func (y *builder) VisitQualifiedNameList(raw javaparser.IQualifiedNameListContext) {
	if y == nil || raw == nil {
		return
	}

	i, _ := raw.(*javaparser.QualifiedNameListContext)
	if i == nil {
		return
	}

}
