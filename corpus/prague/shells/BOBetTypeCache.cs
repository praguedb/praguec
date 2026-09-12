namespace BO.Services.Prague.Models;

public sealed partial class BOBetTypeCache
{
    public void Clear()
    {
        foreach (var item in Query().Execute()) 
        {
            Remove(item.Key);
        }
    }
}