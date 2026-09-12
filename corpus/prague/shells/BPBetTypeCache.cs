namespace Common.IntegratedEventApi.Cache.Prague.Models;

public sealed partial class BPBetTypeCache
{
    public void Clear()
    {
        foreach (var item in Query().Execute()) 
        {
            Remove(item.Key);
        }
    }
}